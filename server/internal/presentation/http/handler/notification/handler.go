// Package notification exposes the customer notification inbox and its SSE
// stream. The recipient always comes from JWT claims.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	notificationapp "github.com/dont-wait/anomaly/internal/application/notification"
	notificationdomain "github.com/dont-wait/anomaly/internal/domain/notification"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/rs/zerolog"
)

type Handler struct {
	queries  *notificationapp.Queries
	commands *notificationapp.Commands
	hub      *notificationapp.Hub
	log      zerolog.Logger
}

func NewHandler(queries *notificationapp.Queries, commands *notificationapp.Commands, hub *notificationapp.Hub, log zerolog.Logger) *Handler {
	return &Handler{queries: queries, commands: commands, hub: hub, log: log}
}

type NotificationDTO struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Category   string            `json:"category"`
	Severity   string            `json:"severity"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Data       map[string]string `json:"data"`
	OccurredAt time.Time         `json:"occurredAt"`
	CreatedAt  time.Time         `json:"createdAt"`
	Read       bool              `json:"read"`
}

type ListResponse struct {
	Items      []NotificationDTO `json:"items"`
	NextCursor string            `json:"nextCursor"`
}

type UnreadCountResponse struct {
	Count int64 `json:"count"`
}

type MarkReadResponse struct {
	ID   string `json:"id"`
	Read bool   `json:"read"`
}

type ReadAllRequest struct {
	Before *time.Time `json:"before,omitempty"`
}

type ReadAllResponse struct {
	Updated int64 `json:"updated"`
}

func toDTO(n notificationdomain.Notification) NotificationDTO {
	data := n.Data
	if data == nil {
		data = map[string]string{}
	}
	return NotificationDTO{
		ID: n.ID, Type: string(n.Type), Category: string(n.Category), Severity: string(n.Severity),
		Title: n.Title, Body: n.Body, Data: data, OccurredAt: n.OccurredAt, CreatedAt: n.CreatedAt,
		Read: n.ReadAt != nil,
	}
}

var errUnauthorized = errors.New("authentication required")

func recipientFrom(r *http.Request) (notificationapp.Recipient, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		return notificationapp.Recipient{}, false
	}
	return notificationapp.Recipient{Audience: notificationdomain.AudienceCustomer, ID: claims.UserID}, true
}

func errorStatus(err error) int {
	switch {
	case errors.Is(err, errUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, notificationdomain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, notificationapp.ErrTooManyConnections):
		return http.StatusTooManyRequests
	case errors.Is(err, notificationdomain.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	httpx.WriteError(w, h.log, err, errorStatus)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	recipient, ok := recipientFrom(r)
	if !ok {
		h.fail(w, errUnauthorized)
		return
	}
	query := notificationapp.ListQuery{Recipient: recipient, Cursor: r.URL.Query().Get("cursor")}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit == 0 {
			h.fail(w, notificationapp.ErrInvalidLimit)
			return
		}
		query.Limit = limit
	}
	if raw := r.URL.Query().Get("unread"); raw != "" {
		unread, err := strconv.ParseBool(raw)
		if err != nil {
			h.fail(w, errors.Join(notificationdomain.ErrInvalid, errors.New("unread must be a boolean")))
			return
		}
		query.UnreadOnly = unread
	}
	page, err := h.queries.List(r.Context(), query)
	if err != nil {
		h.fail(w, err)
		return
	}
	response := ListResponse{Items: make([]NotificationDTO, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		response.Items = append(response.Items, toDTO(item))
	}
	httpx.WriteSuccess(w, http.StatusOK, "notifications", response)
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) {
	recipient, ok := recipientFrom(r)
	if !ok {
		h.fail(w, errUnauthorized)
		return
	}
	count, err := h.queries.CountUnread(r.Context(), recipient)
	if err != nil {
		h.fail(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, "unread count", UnreadCountResponse{Count: count})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	recipient, ok := recipientFrom(r)
	if !ok {
		h.fail(w, errUnauthorized)
		return
	}
	id := r.PathValue("id")
	if err := h.commands.MarkRead(r.Context(), recipient, id); err != nil {
		h.fail(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, "notification marked as read", MarkReadResponse{ID: id, Read: true})
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	recipient, ok := recipientFrom(r)
	if !ok {
		h.fail(w, errUnauthorized)
		return
	}
	var request ReadAllRequest
	if err := decodeOptionalJSON(w, r, &request); err != nil {
		h.fail(w, errors.Join(notificationdomain.ErrInvalid, err))
		return
	}
	var before time.Time
	if request.Before != nil {
		before = *request.Before
	}
	updated, err := h.commands.MarkAllRead(r.Context(), recipient, before)
	if err != nil {
		h.fail(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, "notifications marked as read", ReadAllResponse{Updated: updated})
}

// decodeOptionalJSON accepts an empty body as "use defaults".
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := httpx.DecodeJSON(w, r, v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (h *Handler) countUnread(ctx context.Context, recipient notificationapp.Recipient) ([]byte, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	count, err := h.queries.CountUnread(queryCtx, recipient)
	if err != nil {
		return nil, err
	}
	return json.Marshal(UnreadCountResponse{Count: count})
}
