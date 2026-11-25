package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

const (
	authCookieName = "ffbatch_token"
	authCookieAge  = 30 * 24 * 60 * 60 // 30 days, in seconds
)

// auth enforces an optional shared-secret. When no token is configured the
// dashboard stays loopback-only and no checks run.
func (s *Server) auth(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supplied := ""
		if cookie, err := r.Cookie(authCookieName); err == nil {
			supplied = cookie.Value
		}
		if supplied == "" {
			supplied = r.Header.Get("X-FFBatch-Token")
		}
		if supplied == "" {
			if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
				supplied = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			}
		}
		if supplied == "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			if queryToken := r.URL.Query().Get("token"); queryToken != "" {
				if tokenEqual(queryToken, s.token) {
					http.SetCookie(w, &http.Cookie{
						Name:     authCookieName,
						Value:    s.token,
						Path:     "/",
						HttpOnly: true,
						Secure:   r.TLS != nil,
						SameSite: http.SameSiteStrictMode,
						MaxAge:   authCookieAge,
					})
					http.Redirect(w, r, "/", http.StatusSeeOther)
					return
				}
			}
		}
		if !tokenEqual(supplied, s.token) {
			w.Header().Set("Cache-Control", "no-store")
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusUnauthorized, "dashboard token required")
				return
			}
			http.Error(w, "Unauthorized. Open the dashboard URL printed by ffbatch serve, or append ?token=YOUR_TOKEN.", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenEqual compares fixed-size digests so length differences do not leak
// through timing.
func tokenEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	left := sha256.Sum256([]byte(a))
	right := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

// TokenizedURL adds the dashboard token to a URL for the initial browser open.
func TokenizedURL(base, token string) string {
	if token == "" {
		return base
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
