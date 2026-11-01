package account

import (
	"net/http"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
	"github.com/dont-wait/anomaly/internal/presentation/http/middleware"
	"github.com/dont-wait/anomaly/internal/presentation/http/openapi"
)

func RegisterRoutes(
	router *openapi.Registry,
	h *Handler,
	tokenSvc queries.TokenService,
) {
	router.HandleFunc("POST /api/auth/register", h.Register, openapi.Operation{
		ID:              "registerAccount",
		Summary:         "Register an account",
		Tags:            []string{"Auth"},
		Request:         registerRequest{},
		Response:        httpx.SuccessResponse[RegisterResponse]{},
		SuccessStatus:   http.StatusCreated,
		FailureStatuses: []int{http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusServiceUnavailable},
	})
	router.HandleFunc("POST /api/auth/login", h.Login, openapi.Operation{
		ID:              "login",
		Summary:         "Login with CCCD and password",
		Tags:            []string{"Auth"},
		Request:         loginRequest{},
		Response:        httpx.SuccessResponse[AuthResponse]{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden},
	})
	router.HandleFunc("POST /api/kyc/session", h.StartKYCSession, openapi.Operation{
		ID:              "createKYCSession",
		Summary:         "Resume KYC with account credentials",
		Description:     "Returns a fresh purpose-limited KYC token only for an unverified account.",
		Tags:            []string{"KYC"},
		Request:         loginRequest{},
		Response:        httpx.SuccessResponse[KYCSessionResponse]{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden},
	})

	router.Handle("GET /api/auth/me",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Me)), openapi.Operation{
			ID:              "getCurrentAccount",
			Summary:         "Get the current account",
			Tags:            []string{"Auth"},
			Response:        httpx.SuccessResponse[AccountResponsePrivate]{},
			FailureStatuses: []int{http.StatusUnauthorized, http.StatusNotFound},
			Auth:            true,
		})
	router.Handle("PATCH /api/auth/me/profile", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.UpdateProfile)), openapi.Operation{
		ID: "updateCurrentProfile", Summary: "Update editable fields in the current customer profile", Tags: []string{"Profile"},
		Request: profileUpdateRequest{}, Response: httpx.SuccessResponse[AccountResponsePrivate]{},
		FailureStatuses: []int{400, 401, 404, 409, 500}, Auth: true,
	})
	router.Handle("PUT /api/auth/me/password", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.ChangePassword)), openapi.Operation{
		ID: "changeCurrentPassword", Summary: "Change the current account password", Tags: []string{"Profile"},
		Request: changePasswordRequest{}, Response: httpx.SuccessResponse[map[string]bool]{},
		FailureStatuses: []int{400, 401, 404, 500}, Auth: true,
	})
	router.Handle("POST /api/auth/me/avatar", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.UploadProfileAvatar)), openapi.Operation{
		ID: "uploadCurrentAvatar", Summary: "Upload or replace the current account avatar", Tags: []string{"Profile"},
		Request: profileAvatarUploadRequest{}, RequestContentType: "multipart/form-data", Response: httpx.SuccessResponse[ProfileMediaResponse]{},
		SuccessStatus: http.StatusCreated, FailureStatuses: []int{400, 401, 413, 415, 500, 503}, Auth: true,
	})
	router.Handle("POST /api/auth/me/documents", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.UploadProfileDocument)), openapi.Operation{
		ID: "uploadCurrentProfileDocument", Summary: "Upload or replace optional income/address evidence", Tags: []string{"Profile"},
		Request: profileDocumentUploadRequest{}, RequestContentType: "multipart/form-data", Response: httpx.SuccessResponse[ProfileMediaResponse]{},
		SuccessStatus: http.StatusCreated, FailureStatuses: []int{400, 401, 403, 413, 415, 500, 503}, Auth: true,
	})
	router.Handle("GET /api/auth/me/documents", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.ListProfileDocuments)), openapi.Operation{
		ID: "listCurrentProfileDocuments", Summary: "List current account profile documents", Tags: []string{"Profile"},
		Response: httpx.SuccessResponse[[]ProfileMediaResponse]{}, FailureStatuses: []int{401, 404, 500}, Auth: true,
	})
	router.Handle("GET /api/auth/me/media/{mediaId}", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.DownloadProfileMedia)), openapi.Operation{
		ID: "downloadCurrentProfileMedia", Summary: "Download current account profile media", Tags: []string{"Profile"},
		Parameters: []openapi.Parameter{{Name: "mediaId", In: "path", Required: true, Type: "string"}},
		Response:   openapi.Binary{}, ResponseContentType: "application/octet-stream", FailureStatuses: []int{401, 404, 500, 503}, Auth: true,
	})
	router.Handle("DELETE /api/auth/me/documents/{mediaId}", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.DeleteProfileDocument)), openapi.Operation{
		ID: "deleteCurrentProfileDocument", Summary: "Delete a replaceable profile document", Tags: []string{"Profile"},
		Parameters: []openapi.Parameter{{Name: "mediaId", In: "path", Required: true, Type: "string"}},
		Response:   httpx.SuccessResponse[map[string]bool]{}, FailureStatuses: []int{401, 403, 404, 500, 503}, Auth: true,
	})
	router.Handle("DELETE /api/auth/me/avatar", middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.DeleteProfileAvatar)), openapi.Operation{
		ID: "deleteCurrentAvatar", Summary: "Remove the current account avatar", Tags: []string{"Profile"},
		Response: httpx.SuccessResponse[map[string]bool]{}, FailureStatuses: []int{401, 404, 500, 503}, Auth: true,
	})
	router.Handle("POST /api/kyc/complete",
		middleware.RequireKYCAuth(tokenSvc)(http.HandlerFunc(h.CompleteKYC)), openapi.Operation{
			ID:                 "completeKYC",
			Summary:            "Complete pre-login KYC",
			Description:        "Requires a KYC bearer token. Validates media, calls the configured KYC service, and stores media only after a VERIFIED decision.",
			Tags:               []string{"KYC"},
			Request:            completeKYCRequest{},
			RequestContentType: "multipart/form-data",
			Response:           httpx.SuccessResponse[KYCCompleteResponse]{},
			FailureStatuses:    []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusGone, http.StatusRequestEntityTooLarge, http.StatusBadGateway},
			Auth:               true,
			AuthScheme:         "kycBearerAuth",
			Parameters:         []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}},
		})
	router.Handle("POST /api/kyc/realtime/attempts", middleware.RequireKYCAuth(tokenSvc)(http.HandlerFunc(h.CreateRealtimeAttempt)), openapi.Operation{
		ID: "createRealtimeKYC", Summary: "Create a realtime KYC attempt", Tags: []string{"KYC"},
		Request: createRealtimeRequest{}, RequestContentType: "multipart/form-data", Response: httpx.SuccessResponse[realtimeAttemptResponse]{},
		Parameters:    []openapi.Parameter{{Name: "Idempotency-Key", In: "header", Required: true, Type: "string", Format: "uuid"}},
		SuccessStatus: http.StatusCreated, FailureStatuses: []int{400, 401, 409, 429, 502}, Auth: true, AuthScheme: "kycBearerAuth",
	})
	router.Handle("DELETE /api/kyc/realtime/attempts/{attemptId}", middleware.RequireKYCAuth(tokenSvc)(http.HandlerFunc(h.CancelRealtimeAttempt)), openapi.Operation{
		ID: "cancelRealtimeKYC", Summary: "Cancel realtime KYC", Tags: []string{"KYC"}, Auth: true, AuthScheme: "kycBearerAuth",
		Response: httpx.SuccessResponse[map[string]bool]{}, FailureStatuses: []int{401, 404},
	})
	router.HandleFunc("GET /api/kyc/realtime/attempts/{attemptId}/stream", h.RealtimeStream, openapi.Operation{
		ID: "streamRealtimeKYC", Summary: "Stream camera video over WebSocket; authenticate with one-use ticket in first message", Tags: []string{"KYC"}, FailureStatuses: []int{400, 403, 503},
	})

	adminOnly := func(handler http.Handler) http.Handler {
		return middleware.RequireAuth(tokenSvc)(h.RequireCurrentAdmin(handler))
	}
	router.Handle("GET /api/accounts", adminOnly(http.HandlerFunc(h.GetAll)), openapi.Operation{
		ID:              "listAccounts",
		Summary:         "List all accounts",
		Tags:            []string{"Accounts"},
		Response:        httpx.SuccessResponse[[]AccountResponsePublic]{},
		FailureStatuses: []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError},
		Auth:            true,
	})
	router.Handle("GET /api/accounts/by-email/{email}",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetByEmail)), openapi.Operation{
			ID:              "getAccountByEmail",
			Summary:         "Get an account by email",
			Tags:            []string{"Accounts"},
			Response:        httpx.SuccessResponse[AccountResponsePublic]{},
			FailureStatuses: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
			Auth:            true,
		})
	router.Handle("GET /api/accounts/{id}",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.GetByID)), openapi.Operation{
			ID:              "getAccountByID",
			Summary:         "Get an account by ID",
			Tags:            []string{"Accounts"},
			Response:        httpx.SuccessResponse[AccountResponsePublic]{},
			FailureStatuses: []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
			Auth:            true,
		})
}

type profileAvatarUploadRequest struct {
	File string `json:"file" form:"file" format:"binary"`
}

type profileDocumentUploadRequest struct {
	Type string `json:"type" form:"type" enum:"INCOME_PROOF,ADDRESS_PROOF"`
	File string `json:"file" form:"file" format:"binary"`
}

type completeKYCRequest struct {
	IDCardFront       string `json:"idCardFront" form:"idCardFront" format:"binary"`
	IDCardBack        string `json:"idCardBack" form:"idCardBack" format:"binary"`
	LiveVideo         string `json:"liveVideo" form:"liveVideo" format:"binary"`
	ChallengeType     string `json:"challengeType" form:"challengeType"`
	RealtimeAttemptID string `json:"realtimeAttemptId" form:"realtimeAttemptId"`
}

type createRealtimeRequest struct {
	IDCardFront   string `json:"idCardFront" form:"idCardFront" format:"binary"`
	IDCardBack    string `json:"idCardBack" form:"idCardBack" format:"binary"`
	ChallengeType string `json:"challengeType" form:"challengeType"`
}
