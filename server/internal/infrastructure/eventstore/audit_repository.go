package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

const (
	auditStreamPrefix = "audit-account-"
	auditGlobalStream = "audit-global"
)

type AuditRepository struct {
	client *esdb.Client
}

func NewAuditRepository(client *esdb.Client) *AuditRepository {
	return &AuditRepository{client: client}
}

// Record appends an audit event to an account stream. Events without a known
// account (for example an OTP verification before registration) use the global
// audit stream. The append is optimistic and retries concurrent writers.
func (r *AuditRepository) Record(ctx context.Context, event audit.Event) error {
	if r == nil || r.client == nil {
		return errors.New("audit event store is unavailable")
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Type) == "" || event.SchemaVersion <= 0 || event.OccurredAt.IsZero() {
		return errors.New("invalid audit event")
	}
	eventID, err := uuid.Parse(event.ID)
	if err != nil {
		return fmt.Errorf("invalid audit event id: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	streamID := auditGlobalStream
	if strings.TrimSpace(event.AccountID) != "" {
		streamID = auditStreamPrefix + event.AccountID
	}

	for range 8 {
		revision, found, duplicate, err := r.streamRevision(ctx, streamID, event.ID)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}
		var expected esdb.StreamState = esdb.NoStream{}
		if found {
			expected = esdb.Revision(revision)
		}
		_, err = r.client.AppendToStream(ctx, streamID, esdb.AppendToStreamOptions{StreamState: expected}, esdb.EventData{
			EventID: eventID, EventType: event.Type, ContentType: esdb.ContentTypeJson, Data: data,
		})
		if err == nil {
			return nil
		}
		var esErr *esdb.Error
		if !errors.As(err, &esErr) || esErr.Code() != esdb.ErrorCodeWrongExpectedVersion {
			return err
		}
	}
	return errors.New("could not append audit event after concurrent retries")
}

func (r *AuditRepository) streamRevision(ctx context.Context, streamID, eventID string) (uint64, bool, bool, error) {
	const pageSize = 4096
	opts := esdb.ReadStreamOptions{From: esdb.Start{}}
	var last uint64
	for {
		stream, err := r.client.ReadStream(ctx, streamID, opts, pageSize)
		if err != nil {
			if resourceMissing(err) {
				return 0, false, false, nil
			}
			return 0, false, false, err
		}
		count := 0
		item, err := stream.Recv()
		for {
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				// Depending on the EventStoreDB client/server path, a
				// missing stream can be reported while opening the read or
				// from the first Recv call. Treat both forms as an empty
				// stream so the following append can create it.
				if resourceMissing(err) && count == 0 {
					stream.Close()
					return 0, false, false, nil
				}
				stream.Close()
				return 0, false, false, err
			}
			count++
			last = item.Event.EventNumber
			if item.Event.EventID.String() == eventID {
				stream.Close()
				return last, true, true, nil
			}
			item, err = stream.Recv()
		}
		stream.Close()
		if count < pageSize {
			return last, true, false, nil
		}
		opts = esdb.ReadStreamOptions{From: esdb.Revision(last + 1)}
	}
}
