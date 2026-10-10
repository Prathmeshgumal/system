package auth

import (
	"net/http"
	"strings"

	"relay/internal/httpx"
)

// RequireUser lets a request through only with a valid access token,
// and records the caller's user id in the request context.
//
// No database query: the signature proves the token is ours. The price is that
// a token stays valid until it expires (≤ 15 min) even after logout — see ADR 0001.
func (t *Tokens) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			httpx.WriteError(w, r, httpx.Unauthorized("missing access token"))
			return
		}
		c, err := t.ParseAccess(raw)
		if err != nil {
			httpx.WriteError(w, r, httpx.Unauthorized("invalid or expired access token"))
			return
		}
		next.ServeHTTP(w, r.WithContext(httpx.WithUserID(r.Context(), c.UserID)))
	})
}
