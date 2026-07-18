package observability

import (
	"net/http"
	"strings"
	"sync"

	"github.com/zeromicro/go-zero/rest/httpx"
	restrouter "github.com/zeromicro/go-zero/rest/router"
)

const (
	corsAllowHeaders  = "Content-Type, Origin, X-CSRF-Token, Authorization, AccessToken, Token, Range, X-Request-ID"
	corsExposeHeaders = "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, X-Request-ID"
	corsAllowMethods  = "GET, HEAD, POST, PATCH, PUT, DELETE"
)

// HTTPRouter applies sanitized request logging outside CORS and go-zero's
// native timeout/recovery middleware while delegating routing to go-zero.
type HTTPRouter struct {
	base        httpx.Router
	logFn       LogFunc
	corsOrigins []string

	mu      sync.RWMutex
	handler http.Handler
}

func NewHTTPRouter(logFn LogFunc, corsOrigins ...string) *HTTPRouter {
	base := restrouter.NewRouter()
	r := &HTTPRouter{
		base:        base,
		logFn:       logFn,
		corsOrigins: append([]string(nil), corsOrigins...),
	}
	base.SetNotAllowedHandler(corsNotAllowedHandler(r.corsOrigins))
	r.handler = r.buildHandler(nil)
	return r
}

func (r *HTTPRouter) SetRoutePatterns(patterns []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handler = r.buildHandler(patterns)
}

func (r *HTTPRouter) buildHandler(patterns []string) http.Handler {
	return HTTPMiddleware(r.logFn, patterns...)(corsMiddleware(r.base, r.corsOrigins))
}

func (r *HTTPRouter) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()
	handler.ServeHTTP(w, request)
}

func (r *HTTPRouter) Handle(method, path string, handler http.Handler) error {
	return r.base.Handle(method, path, handler)
}

func (r *HTTPRouter) SetNotFoundHandler(handler http.Handler) {
	r.base.SetNotFoundHandler(handler)
}

func (r *HTTPRouter) SetNotAllowedHandler(handler http.Handler) {
	r.base.SetNotAllowedHandler(handler)
}

func corsMiddleware(next http.Handler, origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w.Header(), r, origins)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsNotAllowedHandler(origins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w.Header(), r, origins)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func setCORSHeaders(header http.Header, r *http.Request, origins []string) {
	header.Add("Vary", "Origin")
	if r.Method == http.MethodOptions {
		header.Add("Vary", "Access-Control-Request-Method")
		header.Add("Vary", "Access-Control-Request-Headers")
	}

	origin := allowedOrigin(r.Header.Get("Origin"), origins)
	if origin == "" {
		return
	}
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Methods", corsAllowMethods)
	header.Set("Access-Control-Allow-Headers", corsAllowHeaders)
	header.Set("Access-Control-Expose-Headers", corsExposeHeaders)
	if origin != "*" {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
	header.Set("Access-Control-Max-Age", "86400")
}

func allowedOrigin(origin string, origins []string) string {
	if len(origins) == 0 {
		return "*"
	}
	origin = strings.ToLower(origin)
	for _, allowed := range origins {
		allowed = strings.ToLower(allowed)
		if allowed == "*" {
			return "*"
		}
		if origin == allowed || strings.HasSuffix(origin, "."+allowed) {
			return origin
		}
	}
	return ""
}
