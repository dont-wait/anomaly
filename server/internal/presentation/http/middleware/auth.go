package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	domainauth "github.com/dont-wait/anomaly/internal/domain/auth"
)

type ctxKey string

const claimsCtxKey ctxKey = "auth.claims"

var ErrMissingAuthHeader = errors.New("missing or malformed Authorization header")

// RequireAuth validates Bearer JWT trên header Authorization, parse token qua
// TokenService, và inject *domainauth.Claims vào context để handler downstream dùng.
// Fail (thiếu header, token sai, hết hạn) -> 401.
func RequireAuth(tokenService queries.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := extractBearer(r.Header.Get("Authorization"))
			if err != nil {
				writeUnauthorized(w)
				return
			}

			claims, err := tokenService.Parse(raw)
			if err != nil {
				writeUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext lấy claims từ context (set bởi RequireAuth).
// Trả về nil, false nếu không có (chưa qua middleware).
func ClaimsFromContext(ctx context.Context) (*domainauth.Claims, bool) {
	c, ok := ctx.Value(claimsCtxKey).(*domainauth.Claims)
	return c, ok
}

func extractBearer(header string) (string, error) {
	if header == "" {
		return "", ErrMissingAuthHeader
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", ErrMissingAuthHeader
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", ErrMissingAuthHeader
	}
	return token, nil
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}
