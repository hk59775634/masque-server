package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type ctxKey int

const idKey ctxKey = 1

// FromContext returns the request correlation id, or empty.
func FromContext(ctx context.Context) string {
	s, _ := ctx.Value(idKey).(string)
	return s
}

// Middleware propagates X-Request-ID (client-supplied or generated) and stores it on the context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = newID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), idKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_randfail"
	}
	return "req_" + hex.EncodeToString(b[:])
}
