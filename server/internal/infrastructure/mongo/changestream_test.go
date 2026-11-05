package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestRunChangeStreamResumesAndRecoversLostHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	token := bson.Raw{5, 0, 0, 0, 0}
	type call struct {
		token   bson.Raw
		startAt *bson.Timestamp
	}
	var calls []call
	watch := func(_ context.Context, resume bson.Raw, startAt *bson.Timestamp) (bson.Raw, error) {
		calls = append(calls, call{resume, startAt})
		switch len(calls) {
		case 1:
			return token, errors.New("network")
		case 2:
			return nil, errors.New("(ChangeStreamHistoryLost) gone")
		default:
			cancel()
			return nil, context.Canceled
		}
	}
	var lost []bool
	RunChangeStream(ctx, watch, time.Millisecond, func(_ error, historyLost bool) { lost = append(lost, historyLost) })
	if len(calls) != 3 || calls[0].token != nil || string(calls[1].token) != string(token) ||
		calls[2].token != nil || calls[2].startAt == nil {
		t.Fatalf("unexpected calls: %+v", calls)
	}
	if len(lost) != 2 || lost[0] || !lost[1] {
		t.Fatalf("unexpected history lost flags: %v", lost)
	}
}
