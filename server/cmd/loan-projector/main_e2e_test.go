//go:build e2e

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/eventstore"
	mongorepo "github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	"github.com/google/uuid"
	esdb "github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCheckpointAheadOfStoreE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	// Khởi tạo EventStoreDB riêng cho bài test.
	container, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image: "eventstore/eventstore:23.10.8-bookworm-slim",
				ExposedPorts: []string{
					"2113/tcp",
				},
				Env: map[string]string{
					"EVENTSTORE_INSECURE":        "true",
					"EVENTSTORE_RUN_PROJECTIONS": "None",
				},
				WaitingFor: wait.ForHTTP("/health/live").
					WithPort("2113/tcp").
					WithStatusCodeMatcher(
						func(status int) bool {
							return status == 200 || status == 204
						},
					).
					WithStartupTimeout(90 * time.Second),
			},
			Started: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}

	port, err := container.MappedPort(ctx, "2113/tcp")
	if err != nil {
		t.Fatal(err)
	}

	client, err := eventstore.NewEventStoreClient(
		&domain.EventStoreConfig{
			EventStoreConnString: fmt.Sprintf(
				"esdb://%s:%s?tls=false",
				host,
				port.Port(),
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		eventstore.Disconnect(client)
	})

	// Ghi một event thật vào EventStoreDB.
	eventID := uuid.New()

	result, err := client.AppendToStream(
		ctx,
		"rv05-checkpoint-"+uuid.NewString(),
		esdb.AppendToStreamOptions{
			StreamState: esdb.NoStream{},
		},
		esdb.EventData{
			EventID:     eventID,
			EventType:   "RV05CheckpointProbe",
			ContentType: esdb.ContentTypeJson,
			Data:        []byte(`{"probe":true}`),
		},
	)
	if err != nil {
		t.Fatalf("append probe event: %v", err)
	}

	// Checkpoint trỏ đến event vừa ghi.
	valid := mongorepo.LoanCheckpoint{
		Commit:  result.CommitPosition,
		Prepare: result.PreparePosition,
		EventID: eventID.String(),
	}

	tests := []struct {
		name        string
		checkpoint  mongorepo.LoanCheckpoint
		wantInvalid bool
	}{
		{
			name:        "matching checkpoint",
			checkpoint:  valid,
			wantInvalid: false,
		},
		{
			name: "wrong event ID",
			checkpoint: mongorepo.LoanCheckpoint{
				Commit:  valid.Commit,
				Prepare: valid.Prepare,
				EventID: uuid.NewString(),
			},
			wantInvalid: true,
		},
		{
			name: "position beyond store",
			checkpoint: mongorepo.LoanCheckpoint{
				Commit:  valid.Commit + 1000000,
				Prepare: valid.Prepare + 1000000,
				EventID: uuid.NewString(),
			},
			wantInvalid: true,
		},
		{
			name: "legacy checkpoint without event ID",
			checkpoint: mongorepo.LoanCheckpoint{
				Commit:  valid.Commit,
				Prepare: valid.Prepare,
			},
			wantInvalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid, err := checkpointAheadOfStore(
				ctx,
				client,
				tt.checkpoint,
			)
			if err != nil {
				t.Fatalf("checkpoint validation: %v", err)
			}

			if invalid != tt.wantInvalid {
				t.Fatalf(
					"invalid = %v, want %v",
					invalid,
					tt.wantInvalid,
				)
			}
		})
	}
}
