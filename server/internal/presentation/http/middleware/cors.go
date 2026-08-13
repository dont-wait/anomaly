package middleware

import "net/http"

// NewCORS returns a CORS middleware that echoes the request Origin only when
// it matches allowedOrigins, sets Vary: Origin, and enables credentials.
// Requests from non-allowed origins get the common headers but no Allow-Origin,
// so browsers will reject them.
func NewCORS(allowedOrigins []string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		set[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if origin := r.Header.Get("Origin"); origin != "" {
				w.Header().Add("Vary", "Origin")
				if _, ok := set[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CORS is a convenience wrapper with an empty allowlist (no credentials, no echoed Origin).
func CORS(next http.Handler) http.Handler {
	return NewCORS(nil)(next)
}
