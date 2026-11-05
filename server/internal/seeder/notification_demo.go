package seeder

import (
	"context"
	"fmt"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain/notification"
)

func init() {
	seeders.register("040_notification_demo", seedNotificationDemo)
}

// demoNotificationSourceID is stable so reseeding never duplicates the item.
const demoNotificationSourceID = "seed:notification-demo:v1"

// seedNotificationDemo gives the demo customer one SYSTEM_TEST notification so
// the inbox, badge and SSE stream can be checked before any module produces
// real notifications. cmd/seed already refuses to run outside development.
func seedNotificationDemo(ctx context.Context, deps Dependencies) error {
	if deps.Notifier == nil || deps.Accounts == nil {
		return nil
	}
	customer, err := deps.Accounts.FindByCCCDNumber(ctx, demoAccounts()[0].CCCD)
	if err != nil {
		return fmt.Errorf("find demo customer: %w", err)
	}
	if customer == nil {
		return nil
	}
	return deps.Notifier.Notify(ctx, notificationapp.Request{
		Audience:      notification.AudienceCustomer,
		RecipientID:   customer.Id,
		Type:          notification.TypeSystemTest,
		SourceEventID: demoNotificationSourceID,
		OccurredAt:    time.Now().UTC(),
		Params:        map[string]string{"message": "Hệ thống thông báo đã sẵn sàng."},
	})
}
