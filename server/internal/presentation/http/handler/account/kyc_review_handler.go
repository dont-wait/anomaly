package account

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dont-wait/anomaly/internal/application/account/commands"
	"github.com/dont-wait/anomaly/internal/application/account/queries"
	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
	"github.com/dont-wait/anomaly/internal/domain/audit"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

const maxKYCMediaBytes = 32 << 20

// KYCReviewIdentity is the identity snapshot shown next to the uploaded
// documents so reviewers can spot mismatches.
type KYCReviewIdentity struct {
	Type             string     `json:"type"`
	Number           string     `json:"number"`
	FullName         string     `json:"fullName"`
	DateOfBirth      *time.Time `json:"dateOfBirth,omitempty"`
	IssuedDate       *time.Time `json:"issuedDate,omitempty"`
	IssuedPlace      string     `json:"issuedPlace,omitempty"`
	PermanentAddress string     `json:"permanentAddress,omitempty"`
}

// KYCReviewMedia lists the evidence endpoints for one submission.
type KYCReviewMedia struct {
	Front     string  `json:"front"`
	Back      string  `json:"back"`
	Video     string  `json:"video"`
	FaceImage *string `json:"faceImage,omitempty"`
}

// KYCReviewItem is one submitted attempt waiting for an administrator.
type KYCReviewItem struct {
	AccountID    string            `json:"accountId"`
	Username     string            `json:"username"`
	Email        string            `json:"email"`
	Phone        string            `json:"phone"`
	SessionID    string            `json:"sessionId"`
	AttemptNo    int               `json:"attemptNo"`
	SubmittedAt  time.Time         `json:"submittedAt"`
	Declared     KYCReviewIdentity `json:"declared"`
	Registration KYCReviewIdentity `json:"registration"`
	Media        KYCReviewMedia    `json:"media"`
}

type kycReviewDecisionRequest struct {
	SessionID string `json:"sessionId"`
	Decision  string `json:"decision" enum:"APPROVED,REJECTED"`
	Reason    string `json:"reason,omitempty"`
}

// ListKYCReviews returns every submission waiting for a manual decision.
func (h *Handler) ListKYCReviews(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.reviewKYC == nil {
		httpx.WriteError(w, h.logger, errors.New("KYC review unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	accounts, err := h.getAll.Handle(r.Context(), queries.GetAllAccountsQuery{})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	items := make([]KYCReviewItem, 0)
	for _, account := range accounts {
		if account == nil || account.Customer == nil || !account.IsKYCUnderReview() {
			continue
		}
		session := account.PendingKYCSession()
		if session == nil {
			continue
		}
		items = append(items, toKYCReviewItem(account, session))
	}
	httpx.WriteSuccess(w, http.StatusOK, "KYC submissions retrieved successfully", items)
}

func toKYCReviewItem(account *accountdomain.UserAccount, session *accountdomain.KYCSession) KYCReviewItem {
	base := "/api/admin/kyc/reviews/" + account.Id + "/media/"
	query := "?sessionId=" + session.Id
	item := KYCReviewItem{
		AccountID:   account.Id,
		Username:    account.Username,
		Email:       account.Email,
		Phone:       account.Customer.Profile.Phone,
		SessionID:   session.Id,
		AttemptNo:   session.AttemptNo,
		SubmittedAt: session.CreatedAt,
		Declared:    toKYCReviewIdentity(session.IdentityData),
		Registration: KYCReviewIdentity{
			Type:             account.Customer.Identity.Type,
			Number:           account.Customer.Identity.Number,
			FullName:         account.Customer.Profile.FullName,
			DateOfBirth:      account.Customer.Profile.DateOfBirth,
			IssuedDate:       account.Customer.Identity.IssuedDate,
			IssuedPlace:      account.Customer.Identity.IssuedPlace,
			PermanentAddress: account.Customer.Identity.PermanentAddress,
		},
		Media: KYCReviewMedia{
			Front: base + "front" + query,
			Back:  base + "back" + query,
			Video: base + "video" + query,
		},
	}
	if session.Media.FaceImage != nil && session.Media.FaceImage.StorageKey != "" {
		face := base + "face" + query
		item.Media.FaceImage = &face
	}
	return item
}

func toKYCReviewIdentity(identity accountdomain.KYCIdentityData) KYCReviewIdentity {
	return KYCReviewIdentity{
		Type:             identity.Type,
		Number:           identity.Number,
		FullName:         identity.FullName,
		DateOfBirth:      identity.DateOfBirth,
		IssuedDate:       identity.IssuedDate,
		IssuedPlace:      identity.IssuedPlace,
		PermanentAddress: identity.PermanentAddress,
	}
}

// DecideKYCReview approves or rejects a submitted attempt.
func (h *Handler) DecideKYCReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httpx.WriteError(w, h.logger, middleware.ErrInvalidToken, func(error) int { return http.StatusUnauthorized })
		return
	}
	if h.reviewKYC == nil {
		httpx.WriteError(w, h.logger, errors.New("KYC review unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	var req kycReviewDecisionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int { return http.StatusBadRequest })
		return
	}
	account, err := h.reviewKYC.Decide(r.Context(), commands.ReviewKYCCommand{
		AdminID:   claims.UserID,
		AccountID: r.PathValue("accountId"),
		SessionID: strings.TrimSpace(req.SessionID),
		Decision:  strings.ToUpper(strings.TrimSpace(req.Decision)),
		Reason:    req.Reason,
	})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	h.recordKYCReviewAudit(r, claims.UserID, account.Id, req.Decision, req.Reason)
	if strings.EqualFold(strings.TrimSpace(req.Decision), accountdomain.KYCReviewRejected) && h.realtime != nil {
		if err := h.realtime.ResetAttempts(r.Context(), account.Id); err != nil {
			h.logger.Warn().Err(err).Msg("reset KYC attempt counter failed")
		}
	}
	httpx.WriteSuccess(w, http.StatusOK, "KYC review recorded", map[string]string{
		"accountId": account.Id,
		"decision":  strings.ToUpper(strings.TrimSpace(req.Decision)),
		"kycStatus": string(account.Customer.KYCStatus),
	})
}

func (h *Handler) recordKYCReviewAudit(r *http.Request, adminID, accountID, decision, reason string) {
	if h.profileAudit == nil {
		return
	}
	eventType := audit.KYCReviewRejected
	if strings.EqualFold(strings.TrimSpace(decision), accountdomain.KYCReviewApproved) {
		eventType = audit.KYCReviewApproved
	}
	event := audit.NewEvent(eventType, r.Header.Get("X-Request-ID"), time.Now().UTC())
	event.AccountID = accountID
	event.ActorID = adminID
	event.ReasonCode = strings.ToUpper(strings.TrimSpace(decision))
	event.Reason = strings.TrimSpace(reason)
	if err := h.profileAudit.Record(r.Context(), event); err != nil {
		h.logger.Warn().Err(err).Str("event", eventType).Msg("record KYC review audit failed")
	}
}

// DownloadKYCReviewMedia streams one evidence object to an administrator.
// The customer-facing media endpoint intentionally refuses pending sessions.
func (h *Handler) DownloadKYCReviewMedia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.media == nil {
		httpx.WriteError(w, h.logger, errors.New("KYC media storage unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	account, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: r.PathValue("accountId")})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if account == nil || account.Customer == nil {
		httpx.WriteError(w, h.logger, accountdomain.ErrAccountNotFound, accountErrorStatus, accountErrorCode)
		return
	}
	session := reviewSession(account, r.URL.Query().Get("sessionId"))
	if session == nil {
		httpx.WriteError(w, h.logger, errors.New("KYC submission not found"), func(error) int { return http.StatusNotFound })
		return
	}
	media, filename, ok := kycReviewMedia(session, r.PathValue("kind"))
	if !ok {
		httpx.WriteError(w, h.logger, errors.New("unsupported KYC media kind"), func(error) int { return http.StatusNotFound })
		return
	}
	contentType := media.MIMEType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !allowedKYCMediaType(contentType) {
		httpx.WriteError(w, h.logger, errors.New("unsupported stored KYC media type"), func(error) int { return http.StatusUnsupportedMediaType })
		return
	}
	downloader, ok := h.media.(profileMediaDownloader)
	if !ok {
		httpx.WriteError(w, h.logger, errors.New("KYC media download unavailable"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	result, err := downloader.Download(r.Context(), media.StorageKey)
	if err != nil {
		httpx.WriteError(w, h.logger, err, func(error) int {
			if errors.Is(err, rustfs.ErrObjectNotFound) {
				return http.StatusNotFound
			}
			return http.StatusServiceUnavailable
		})
		return
	}
	defer func() { _ = result.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(result.Body, maxKYCMediaBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxKYCMediaBytes {
		httpx.WriteError(w, h.logger, errors.New("KYC media could not be read"), func(error) int { return http.StatusServiceUnavailable })
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filename, mediaTime(session), bytes.NewReader(data))
}

func reviewSession(account *accountdomain.UserAccount, sessionID string) *accountdomain.KYCSession {
	if sessionID != "" {
		for _, session := range account.KYCSessions {
			if session != nil && session.Id == sessionID {
				return session
			}
		}
		return nil
	}
	if pending := account.PendingKYCSession(); pending != nil {
		return pending
	}
	return account.LatestKYCSession()
}

func kycReviewMedia(session *accountdomain.KYCSession, kind string) (accountdomain.MediaObject, string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "front":
		return session.Media.IdentityFront, "identity-front" + extensionFor(session.Media.IdentityFront), session.Media.IdentityFront.StorageKey != ""
	case "back":
		return session.Media.IdentityBack, "identity-back" + extensionFor(session.Media.IdentityBack), session.Media.IdentityBack.StorageKey != ""
	case "video":
		media := session.Media.LivenessVideo.MediaObject
		return media, "liveness-video" + extensionFor(media), media.StorageKey != ""
	case "face":
		if session.Media.FaceImage == nil {
			return accountdomain.MediaObject{}, "", false
		}
		return *session.Media.FaceImage, "face-image" + extensionFor(*session.Media.FaceImage), session.Media.FaceImage.StorageKey != ""
	default:
		return accountdomain.MediaObject{}, "", false
	}
}

func extensionFor(media accountdomain.MediaObject) string {
	switch media.MIMEType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	default:
		return ""
	}
}

func allowedKYCMediaType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "video/mp4", "video/webm":
		return true
	default:
		return false
	}
}

func mediaTime(session *accountdomain.KYCSession) time.Time {
	if session.CompletedAt != nil {
		return *session.CompletedAt
	}
	return session.CreatedAt
}
