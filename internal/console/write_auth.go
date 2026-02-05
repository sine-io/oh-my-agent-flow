package console

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type WriteAuthConfig struct {
	SessionToken   string
	AllowedOrigins []string
}

func GenerateSessionToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func RequireWriteAuth(next http.Handler, cfg WriteAuthConfig) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" || origin == "null" {
			WriteAPIError(w, http.StatusForbidden, APIError{
				Code:    "ORIGIN_REQUIRED",
				Message: "Origin header is required for write operations.",
				Hint:    "Open the console UI at http://127.0.0.1 and retry from the same page (not a different origin).",
			})
			return
		}
		if _, ok := allowed[origin]; !ok {
			WriteAPIError(w, http.StatusForbidden, APIError{
				Code:    "ORIGIN_NOT_ALLOWED",
				Message: "Origin is not allowed for write operations.",
				Hint:    "Use the console UI that was started by this server and avoid cross-site requests.",
			})
			return
		}

		token := r.Header.Get("X-Session-Token")
		if token == "" {
			WriteAPIError(w, http.StatusForbidden, APIError{
				Code:    "SESSION_TOKEN_REQUIRED",
				Message: "Session token is required for write operations.",
				Hint:    "Reload the console UI page to receive a fresh session token, then retry.",
			})
			return
		}
		if cfg.SessionToken == "" || token != cfg.SessionToken {
			WriteAPIError(w, http.StatusForbidden, APIError{
				Code:    "SESSION_TOKEN_INVALID",
				Message: "Session token is invalid.",
				Hint:    "Reload the console UI page to receive a new token, then retry.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
