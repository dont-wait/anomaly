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
		Response:        httpx.SuccessResponse[AccountResponsePrivate]{},
		SuccessStatus:   http.StatusCreated,
		FailureStatuses: []int{http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusServiceUnavailable},
	})
	router.HandleFunc("POST /api/auth/login", h.Login, openapi.Operation{
		ID:              "login",
		Summary:         "Login with CCCD and password",
		Tags:            []string{"Auth"},
		Request:         loginRequest{},
		Response:        httpx.SuccessResponse[AuthResponse]{},
		FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized},
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
	router.Handle("POST /api/accounts/{id}/verify",
		middleware.RequireAuth(tokenSvc)(http.HandlerFunc(h.Verify)), openapi.Operation{
			ID:              "verifyAccount",
			Summary:         "Submit KYC media for verification",
			Tags:            []string{"Accounts"},
			Request:         verifyRequest{},
			Response:        httpx.SuccessResponse[AccountResponsePrivate]{},
			FailureStatuses: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound},
			Auth:            true,
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
