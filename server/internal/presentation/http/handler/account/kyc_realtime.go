package account

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"

	"github.com/dont-wait/anomaly/internal/application/account/queries"

	kycinfra "github.com/dont-wait/anomaly/internal/infrastructure/kyc"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
)

var realtimeSockets = make(chan struct{}, 32)

type realtimeAttemptResponse struct {
	AttemptID             string    `json:"attemptId"`
	ExpiresAt             time.Time `json:"expiresAt"`
	StreamPath            string    `json:"streamPath"`
	StreamTicket          string    `json:"streamTicket"`
	StreamTicketExpiresAt time.Time `json:"streamTicketExpiresAt"`
	MimeTypes             []string  `json:"mimeTypes"`
	AttemptLimit          int       `json:"attemptLimit"`
	AttemptsRemaining     int       `json:"attemptsRemaining"`
}

func (h *Handler) CreateRealtimeAttempt(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		h.kycRequestError(w, "unauthorized", 401)
		return
	}
	if h.realtime == nil {
		writeKYCSystemError(w, "Realtime verification is unavailable")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if _, err := uuid.Parse(key); err != nil {
		h.kycRequestError(w, "invalid idempotency key", 400)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 22<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		h.kycRequestError(w, "invalid or oversized multipart", 400)
		return
	}
	defer r.MultipartForm.RemoveAll()
	front, err := readKYCUpload(r.MultipartForm, "idCardFront", maxImageBytes, true)
	if err != nil {
		h.kycRequestError(w, "invalid front image", 400)
		return
	}
	back, err := readKYCUpload(r.MultipartForm, "idCardBack", maxImageBytes, true)
	if err != nil {
		h.kycRequestError(w, "invalid back image", 400)
		return
	}
	if r.FormValue("challengeType") != "TURN_HEAD_LEFT_RIGHT_BLINK" {
		h.kycRequestError(w, "unsupported challenge", 400)
		return
	}
	account, err := h.getByID.Handle(r.Context(), queries.GetAccountByIDQuery{ID: claims.UserID})
	if err != nil {
		httpx.WriteError(w, h.logger, err, accountErrorStatus, accountErrorCode)
		return
	}
	if account.IsVerified() {
		h.kycRequestError(w, "account already verified", 409)
		return
	}
	expiresAt := time.Now().Add(h.realtime.Limits().AttemptTTL)
	if claims.ExpiresAt.Before(expiresAt) {
		expiresAt = claims.ExpiresAt
	}
	if time.Until(expiresAt) < 30*time.Second {
		h.kycRequestError(w, "KYC token is expiring; resume verification", 401)
		return
	}
	a, err := h.realtime.Create(r.Context(), &kycinfra.Attempt{
		AccountID: claims.UserID, Key: key,
		Front:     kycinfra.Media{Filename: front.filename, ContentType: front.contentType, Data: front.data},
		Back:      kycinfra.Media{Filename: back.filename, ContentType: back.contentType, Data: back.data},
		FrontHash: kycinfra.Digest(front.data), BackHash: kycinfra.Digest(back.data),
		Challenge: r.FormValue("challengeType"), ExpiresAt: expiresAt,
	})
	if err != nil {
		status := 502
		if errors.Is(err, kycinfra.ErrAttemptConflict) {
			status = 409
		}
		if errors.Is(err, kycinfra.ErrAttemptLimit) {
			status = 429
		}
		if errors.Is(err, kycinfra.ErrAttemptCapacity) {
			status = 429
		}
		httpx.WriteError(w, h.logger, err, func(error) int { return status }, func(error) httpx.ErrorCode {
			if errors.Is(err, kycinfra.ErrAttemptLimit) {
				return httpx.ErrorCode("KYC_ATTEMPTS_EXCEEDED")
			}
			return httpx.ErrorCodeForStatus(status)
		})
		return
	}
	if len(a.Result) > 0 {
		h.kycRequestError(w, "attempt already completed", 409)
		return
	}
	ticket := uuid.NewString() + uuid.NewString()
	if err = h.realtime.Ticket(r.Context(), a, ticket); err != nil {
		writeKYCSystemError(w, "Realtime verification is unavailable")
		return
	}
	count, _, err := h.realtime.Count(r.Context(), a.AccountID)
	if err != nil {
		writeKYCSystemError(w, "KYC policy is unavailable")
		return
	}
	httpx.WriteSuccess(w, 201, "Realtime attempt created", realtimeAttemptResponse{
		AttemptID: a.ID, ExpiresAt: a.ExpiresAt, AttemptsRemaining: max(0, h.realtime.Limits().MaxAttempts-count), AttemptLimit: h.realtime.Limits().MaxAttempts,
		StreamPath: "/api/kyc/realtime/attempts/" + a.ID + "/stream", StreamTicket: ticket, StreamTicketExpiresAt: time.Now().Add(30 * time.Second), MimeTypes: []string{"video/webm;codecs=vp8", "video/webm"},
	})
}

func (h *Handler) kycRequestError(w http.ResponseWriter, message string, status int) {
	httpx.WriteError(w, h.logger, errors.New(message), func(error) int { return status })
}

func (h *Handler) CancelRealtimeAttempt(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok || claims == nil || h.realtime == nil {
		h.kycRequestError(w, "unauthorized", 401)
		return
	}
	a, err := h.realtime.Get(r.Context(), r.PathValue("attemptId"))
	if err != nil || a.AccountID != claims.UserID {
		h.kycRequestError(w, "KYC attempt not found", 404)
		return
	}
	if len(a.Result) == 0 {
		lock, e := h.realtime.Lock(r.Context(), a.ID)
		if e != nil || lock == "" {
			h.kycRequestError(w, "KYC attempt is processing", 409)
			return
		}
		defer h.realtime.Unlock(context.WithoutCancel(r.Context()), a.ID, lock)
		if err := h.realtime.DB.Set(r.Context(), "kyc:cancelled:"+a.ID, "1", h.realtime.Limits().AttemptTTL).Err(); err != nil {
			writeKYCSystemError(w, "KYC cancellation is unavailable")
			return
		}
		if err := h.realtime.DB.Del(r.Context(), "kyc:attempt:"+a.ID).Err(); err != nil {
			writeKYCSystemError(w, "KYC cancellation is unavailable")
			return
		}
		h.realtime.Release(r.Context(), a)
	}
	httpx.WriteSuccess(w, 200, "Attempt cancelled", map[string]bool{"cancelled": true})
}

func (h *Handler) RealtimeStream(w http.ResponseWriter, r *http.Request) {
	if h.realtime == nil {
		h.kycRequestError(w, "unavailable", 503)
		return
	}
	select {
	case realtimeSockets <- struct{}{}:
		defer func() { <-realtimeSockets }()
	default:
		h.kycRequestError(w, "capacity exceeded", 503)
		return
	}
	server := websocket.Server{Handshake: func(_ *websocket.Config, r *http.Request) error {
		if !h.realtimeOrigins[r.Header.Get("Origin")] {
			return fmt.Errorf("origin not allowed")
		}
		return nil
	}, Handler: func(browser *websocket.Conn) { h.streamAttempt(browser, r) }}
	server.ServeHTTP(w, r)
}

func (h *Handler) streamAttempt(browser *websocket.Conn, r *http.Request) {
	defer browser.Close()
	browser.MaxPayloadBytes = 4096
	_ = browser.SetDeadline(time.Now().Add(5 * time.Second))
	var auth struct {
		Type   string `json:"type"`
		Ticket string `json:"ticket"`
	}
	if err := websocket.JSON.Receive(browser, &auth); err != nil || auth.Type != "authenticate" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id := r.PathValue("attemptId")
	if err := h.realtime.ConsumeTicket(ctx, id, auth.Ticket); err != nil {
		return
	}
	browser.MaxPayloadBytes = (1 << 20) + 5
	a, err := h.realtime.Get(ctx, id)
	if err != nil || time.Now().After(a.ExpiresAt) || len(a.Result) > 0 {
		return
	}
	locked, err := h.realtime.DB.SetNX(ctx, "kyc:stream:"+id, "1", 2*time.Minute).Result()
	if err != nil || !locked {
		return
	}
	streamFinished := false
	defer func() {
		if !streamFinished {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			h.realtime.DB.Del(cleanup, "kyc:attempt:"+id)
			h.realtime.Release(cleanup, a)
		}
	}()
	client, ok := h.kyc.(*kycinfra.Client)
	if !ok {
		return
	}
	backend, err := client.OpenRealtime(ctx, a.Front, a.Challenge)
	if err != nil {
		code := "REALTIME_UNAVAILABLE"
		var analysisErr *kycinfra.RealtimeError
		if errors.As(err, &analysisErr) && (analysisErr.Code == "CCCD_PORTRAIT_NOT_FOUND" || analysisErr.Code == "CCCD_IMAGE_QUALITY_LOW") {
			code = analysisErr.Code
		}
		websocket.JSON.Send(browser, map[string]any{"type": "error", "code": code, "attemptId": id, "eventSeq": 1})
		h.realtime.Release(ctx, a)
		return
	}
	defer backend.Close()
	_ = browser.SetDeadline(a.ExpiresAt)
	_ = backend.SetDeadline(a.ExpiresAt)
	var sendMu sync.Mutex
	eventSeq := 0
	send := func(event map[string]any) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		eventSeq++
		event["attemptId"] = id
		event["eventSeq"] = eventSeq
		return websocket.JSON.Send(browser, event)
	}
	if send(map[string]any{"type": "ready"}) != nil {
		return
	}
	var start struct {
		Type     string `json:"type"`
		MimeType string `json:"mimeType"`
	}
	if websocket.JSON.Receive(browser, &start) != nil || start.Type != "start" || (start.MimeType != "video/webm;codecs=vp8" && start.MimeType != "video/webm") {
		return
	}
	_ = browser.SetDeadline(time.Now().Add(25 * time.Second))
	_ = backend.SetDeadline(time.Now().Add(25 * time.Second))
	if websocket.JSON.Send(backend, map[string]string{"type": "start"}) != nil {
		return
	}
	if send(map[string]any{"type": "started"}) != nil {
		return
	}
	var hashMu sync.Mutex
	digest := sha256.New()
	ended := false
	lastSeq := -1
	total := 0
	ingestErr := make(chan error, 1)
	go func() {
		for {
			var data []byte
			if err := websocket.Message.Receive(browser, &data); err != nil {
				ingestErr <- err
				backend.Close()
				return
			}
			if len(data) > 0 && data[0] == 1 {
				seq := -1
				if len(data) >= 6 {
					seq = int(binary.BigEndian.Uint32(data[1:5]))
				}
				if seq != lastSeq+1 || len(data) < 6 {
					ingestErr <- fmt.Errorf("invalid sequence")
					backend.Close()
					return
				}
				total += len(data) - 5
				if total > maxVideoBytes {
					ingestErr <- fmt.Errorf("video too large")
					backend.Close()
					return
				}
				lastSeq = seq
				hashMu.Lock()
				digest.Write(data[5:])
				hashMu.Unlock()
				if err := websocket.Message.Send(backend, data); err != nil {
					ingestErr <- err
					backend.Close()
					return
				}
				if err := send(map[string]any{"type": "ack", "chunkSeq": seq}); err != nil {
					backend.Close()
					return
				}
			} else {
				var end struct {
					Type string `json:"type"`
					Last int    `json:"lastChunkSeq"`
				}
				if json.Unmarshal(data, &end) != nil || end.Type != "end" || end.Last != lastSeq || lastSeq < 0 {
					ingestErr <- fmt.Errorf("invalid end")
					backend.Close()
					return
				}
				hashMu.Lock()
				ended = true
				hashMu.Unlock()
				if err := websocket.Message.Send(backend, string(data)); err != nil {
					ingestErr <- err
					backend.Close()
					return
				}
				ingestErr <- nil
				return
			}
		}
	}()
	defer browser.Close()
	for {
		var event map[string]any
		if err := websocket.JSON.Receive(backend, &event); err != nil {
			send(map[string]any{"type": "error", "code": "REALTIME_UNAVAILABLE"})
			h.realtime.Release(ctx, a)
			return
		}
		kind, _ := event["type"].(string)
		if kind == "stream_finished" {
			hashMu.Lock()
			finished := ended
			streamHash := hex.EncodeToString(digest.Sum(nil))
			hashMu.Unlock()
			if !finished {
				return
			}
			if err := <-ingestErr; err != nil {
				return
			}
			cancelled, e := h.realtime.DB.Exists(ctx, "kyc:cancelled:"+id).Result()
			if e != nil || cancelled > 0 {
				return
			}
			a.StreamHash = streamHash
			a.Front.Data = nil
			a.Back.Data = nil
			if err := h.realtime.Save(ctx, a); err != nil {
				send(map[string]any{"type": "error", "code": "REALTIME_UNAVAILABLE"})
				return
			}
			streamFinished = true
			send(map[string]any{"type": "stream_finished"})
			return
		}
		// Never expose the internal model scores/evidence to the browser.
		if kind == "progress" || kind == "error" {
			delete(event, "result")
			if send(event) != nil {
				return
			}
			if kind == "error" {
				h.realtime.Release(ctx, a)
				return
			}
		}
	}
}
