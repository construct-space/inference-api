package middleware

import (
	"log"
	"net/http"
	"time"

	"construct/inference/internal/config"
)

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// isPublicReadPath identifies routes that serve the gateway's public
// catalog — no auth, no per-user data, safe to expose to any origin
// (the desktop app's Tauri webview ships with origin tauri://localhost,
// the dev server runs on :60200, future browser tools may show up on
// any host). Listing them here lets CORS short-circuit the
// AllowedOrigins allow-list for endpoints that don't carry secrets.
func isPublicReadPath(method, path string) bool {
	if method != http.MethodGet && method != http.MethodOptions {
		return false
	}
	switch path {
	case "/api/models", "/v1/models", "/api/health", "/health":
		return true
	}
	return false
}

func CORS(cfg *config.Config) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range cfg.AllowedOrigins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			switch {
			case isPublicReadPath(r.Method, r.URL.Path):
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			case allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Internal-Secret")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
