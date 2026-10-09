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
	router.Handle("POST /api/kyc/complete",
		middleware.RequireKYCAuth(tokenSvc)(http.HandlerFunc(h.CompleteKYC)), openapi.Operation{
			ID:                 "completeKYC",
			Summary:            "Complete pre-login KYC",
			Description:        "Requires a KYC bearer token. Validates media, calls the configured KYC service, and stores media only after a VERIFIED decision.",
			Tags:               []string{"KYC"},
			Request:            completeKYCRequest{},
			RequestContentType: "multipart/form-data",
			Response:           httpx.SuccessResponse[KYCCompleteResponse]{},
			FailureStatuses:    []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusBadGateway},
			Auth:               true,
			AuthScheme:         "kycBearerAuth",
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

type completeKYCRequest struct {
	IDCardFront   string `json:"idCardFront" form:"idCardFront" format:"binary"`
	IDCardBack    string `json:"idCardBack" form:"idCardBack" format:"binary"`
	LiveVideo     string `json:"liveVideo" form:"liveVideo" format:"binary"`
	ChallengeType string `json:"challengeType" form:"challengeType"`
}
