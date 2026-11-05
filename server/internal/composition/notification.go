package composition

import (
	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	"github.com/dont-wait/anomaly/internal/domain"
	"github.com/dont-wait/anomaly/internal/infrastructure/mongo"
	notificationhandler "github.com/dont-wait/anomaly/internal/presentation/http/handler/notification"
	"github.com/rs/zerolog"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
)

func notificationAppConfig(config *domain.NotificationConfig) notificationapp.Config {
	return notificationapp.Config{Retention: config.Retention, MaxEventAge: config.MaxEventAge}
}

// NewNotifier tạo Notifier để inject vào module phát thông báo ngoài stream
// banking (vd risk consumer trong cmd/api gọi sau khi lưu assessment).
// Xem docs/how-to-register-noti.md, cách 2.
func NewNotifier(client *mongodrv.Client, dbName string, config *domain.NotificationConfig) *notificationapp.Service {
	return notificationapp.NewService(mongo.NewNotificationRepository(client, dbName), notificationAppConfig(config))
}

// NewNotificationDispatcher là NƠI ĐĂNG KÝ EventMapper cho stream `banking`,
// được cmd/notifier dùng. Thêm mapper của module vào danh sách bên dưới.
// Xem docs/how-to-register-noti.md, cách 1.
func NewNotificationDispatcher(client *mongodrv.Client, dbName string, config *domain.NotificationConfig) *notificationapp.Dispatcher {
	mappers := []notificationapp.EventMapper{
		// Đăng ký mapper của module ở đây, vd:
		// transfernoti.NewMapper(mongo.NewAccountRepository(client, dbName)),
	}
	return notificationapp.NewDispatcher(NewNotifier(client, dbName, config), mappers...)
}

// NotificationAPI bundles what cmd/api needs: the HTTP handler, the hub fed
// by the change stream, and the repository the stream watches.
type NotificationAPI struct {
	Handler    *notificationhandler.Handler
	Hub        *notificationapp.Hub
	Repository *mongo.NotificationRepository
}

func NewNotificationAPI(client *mongodrv.Client, dbName string, config *domain.NotificationConfig, log zerolog.Logger) NotificationAPI {
	repository := mongo.NewNotificationRepository(client, dbName)
	hub := notificationapp.NewHub(config.StreamMaxConnPerUser)
	handler := notificationhandler.NewHandler(notificationapp.NewQueries(repository), notificationapp.NewCommands(repository), hub, log)
	return NotificationAPI{Handler: handler, Hub: hub, Repository: repository}
}
