package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/observability"
)

// instrument records RED metrics for every request, labelling by the matched
// chi route pattern so cardinality stays bounded.
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := chiRoutePattern(r)
		observability.HTTPDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
		observability.HTTPRequests.WithLabelValues(route, r.Method, statusClass(ww.Status())).Inc()
	})
}

// bearerAuth guards write endpoints with a constant-time bearer token check.
// When no token is configured the guard is disabled (useful for local dev).
func bearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func chiRoutePattern(r *http.Request) string {
	if rc := middleware.GetReqID(r.Context()); rc != "" { //nolint:staticcheck // keep request id flowing
		_ = rc
	}
	ctx := r.Context()
	if p := routePatternFromCtx(ctx); p != "" {
		return p
	}
	return "unknown"
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}
