package httpserver

import (
	"context"
	"net/http"
	"time"
)

// Ready only after startup migrations and while the database is reachable.
// Do not return connection strings or database error details to clients.
func Readiness(ping func(context.Context) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte("{\"status\":\"unavailable\"}\n"))
			}
			return
		}
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
		}
	})
}
