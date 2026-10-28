package account

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

type cleanupMedia struct {
	deleted []string
	fail    bool
}

func (s *cleanupMedia) Upload(context.Context, string, io.Reader, string) error { return nil }
func (s *cleanupMedia) Delete(_ context.Context, key string) error {
	if s.fail {
		return errors.New("storage unavailable")
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func TestReconcileKYCMediaPreservesCommittedSessionAndRetriesStorageFailure(t *testing.T) {
	media := &cleanupMedia{}
	account := &accountdomain.UserAccount{KYCSessions: []*accountdomain.KYCSession{{Media: accountdomain.KYCMedia{
		IdentityFront: accountdomain.MediaObject{StorageKey: "official/front"}, IdentityBack: accountdomain.MediaObject{StorageKey: "official/back"},
		LivenessVideo: accountdomain.LivenessVideo{MediaObject: accountdomain.MediaObject{StorageKey: "official/video"}},
	}}}}
	err := reconcileKYCMedia(context.Background(), media, account, []string{"official/front", "orphan/front", "official/video"})
	if err != nil || !reflect.DeepEqual(media.deleted, []string{"orphan/front"}) {
		t.Fatalf("reconciliation deleted official media: %v/%v", media.deleted, err)
	}
	media.fail = true
	if err := reconcileKYCMedia(context.Background(), media, account, []string{"orphan/back"}); err == nil {
		t.Fatal("storage failure acknowledged as cleaned")
	}
}
