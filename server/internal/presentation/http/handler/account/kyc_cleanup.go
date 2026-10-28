package account

import (
	"context"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

// RunKYCStorageCleanup reconciles uncertain storage/DB outcomes against the
// canonical account before deleting anything. Entries survive API restarts.
func (h *Handler) RunKYCStorageCleanup(ctx context.Context) {
	if h.realtime == nil || h.media == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 30*time.Second)
			records, err := h.realtime.PendingCleanup(work)
			if err != nil {
				h.logger.Error().Err(err).Msg("read KYC media reconciliation queue failed")
				cancel()
				continue
			}
			for _, record := range records {
				account, err := h.getByID.Handle(work, queries.GetAccountByIDQuery{ID: record.AccountID})
				if err != nil {
					continue
				}
				if err := reconcileKYCMedia(work, h.media, account, record.Keys); err != nil {
					continue
				}
				if err := h.realtime.FinishCleanup(work, record.ID); err != nil {
					h.logger.Error().Err(err).Msg("acknowledge KYC reconciliation failed")
				}
			}
			cancel()
		}
	}
}

func reconcileKYCMedia(ctx context.Context, media MediaStore, account *accountdomain.UserAccount, keys []string) error {
	referenced := map[string]bool{}
	for _, session := range account.KYCSessions {
		referenced[session.Media.IdentityFront.StorageKey] = true
		referenced[session.Media.IdentityBack.StorageKey] = true
		referenced[session.Media.LivenessVideo.StorageKey] = true
		if session.Media.FaceImage != nil {
			referenced[session.Media.FaceImage.StorageKey] = true
		}
	}
	for _, key := range keys {
		if !referenced[key] {
			if err := media.Delete(ctx, key); err != nil {
				return err
			}
		}
	}
	return nil
}
