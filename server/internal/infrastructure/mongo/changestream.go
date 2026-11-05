package mongo

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// WatchFunc runs one change stream session until it stops and returns the
// last resume token it saw.
type WatchFunc func(ctx context.Context, resumeAfter bson.Raw, startAtOperationTime *bson.Timestamp) (bson.Raw, error)

// RunChangeStream keeps a change stream alive until ctx is done. It resumes
// from the last token; when the oplog no longer has that token it restarts
// from "now". afterStop runs after every stop (before the retry delay) so
// callers can log and replay anything a stream gap may have skipped.
func RunChangeStream(ctx context.Context, watch WatchFunc, retryDelay time.Duration, afterStop func(err error, historyLost bool)) {
	var resumeToken bson.Raw
	var startAtOperationTime *bson.Timestamp
	for ctx.Err() == nil {
		nextToken, err := watch(ctx, resumeToken, startAtOperationTime)
		if len(nextToken) > 0 {
			resumeToken = append(bson.Raw(nil), nextToken...)
		}
		if ctx.Err() != nil {
			return
		}
		historyLost := IsChangeStreamHistoryLost(err)
		var recoveryStart *bson.Timestamp
		if historyLost {
			recoveryStart = &bson.Timestamp{T: uint32(time.Now().Unix())}
		}
		if afterStop != nil {
			afterStop(err, historyLost)
		}
		if historyLost {
			resumeToken = nil
			startAtOperationTime = recoveryStart
		} else if len(nextToken) > 0 {
			startAtOperationTime = nil
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func IsChangeStreamHistoryLost(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "changestreamhistorylost") || strings.Contains(message, "resume token")
}
