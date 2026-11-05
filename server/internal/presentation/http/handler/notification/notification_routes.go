package notification

import (
	"net/http"

	accountqueries "github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func RegisterRoutes(router *openapi.Registry, h *Handler, tokens accountqueries.TokenService) {
	if router == nil || h == nil || tokens == nil {
		return
	}
	auth := func(pattern string, fn http.HandlerFunc, op openapi.Operation) {
		op.Auth = true
		op.Tags = []string{"Notifications"}
		op.FailureResponses = map[int]any{}
		for _, status := range op.FailureStatuses {
			op.FailureResponses[status] = httpx.ErrorResponse{}
		}
		router.Handle(pattern, middleware.RequireAuth(tokens)(fn), op)
	}
	auth("GET /api/notifications", h.list, openapi.Operation{
		ID: "listNotifications", Summary: "List the current user's notifications, newest first",
		Parameters: []openapi.Parameter{
			{Name: "cursor", In: "query", Type: "string"},
			{Name: "limit", In: "query", Type: "integer"},
			{Name: "unread", In: "query", Type: "boolean"},
		},
		Response: httpx.SuccessResponse[ListResponse]{}, FailureStatuses: []int{400, 401},
	})
	auth("GET /api/notifications/unread-count", h.unreadCount, openapi.Operation{
		ID: "countUnreadNotifications", Summary: "Count unread notifications for the badge",
		Response: httpx.SuccessResponse[UnreadCountResponse]{}, FailureStatuses: []int{401},
	})
	auth("PATCH /api/notifications/{id}/read", h.markRead, openapi.Operation{
		ID: "markNotificationRead", Summary: "Mark one notification as read",
		Description: "Idempotent. Returns 404 when the id does not exist or belongs to someone else.",
		Response:    httpx.SuccessResponse[MarkReadResponse]{}, FailureStatuses: []int{401, 404},
	})
	auth("POST /api/notifications/read-all", h.readAll, openapi.Operation{
		ID: "markAllNotificationsRead", Summary: "Mark every notification created at or before `before` as read",
		Request: ReadAllRequest{}, Response: httpx.SuccessResponse[ReadAllResponse]{}, FailureStatuses: []int{400, 401},
	})
	auth("GET /api/notifications/stream", h.stream, openapi.Operation{
		ID: "streamNotifications", Summary: "Server-Sent Events stream of new notifications",
		Description:         "Events: `notification` (id is the resume cursor) and `unread-count`. Resume with `?cursor=` or `Last-Event-ID`. Heartbeat comment every 15s.",
		Parameters:          []openapi.Parameter{{Name: "cursor", In: "query", Type: "string"}},
		ResponseContentType: "text/event-stream", FailureStatuses: []int{400, 401, 429},
	})
}
