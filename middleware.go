package main

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// ================= Middleware =================

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingMiddleware logs request details
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.ActiveConnections.Add(1)
		defer metrics.ActiveConnections.Add(-1)

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %v", r.Method, r.URL.Path, rw.statusCode, duration)

		metrics.RecordRequest(r.URL.Path, duration.Milliseconds(), rw.statusCode >= http.StatusBadRequest)
	})
}

func apiKeyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || !isProtectedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.Header.Get("x-api-key")
		}
		if auth == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			auth = "Bearer " + auth
		}
		r.Header.Set("Authorization", auth)

		bearerToken := strings.TrimPrefix(auth, "Bearer ")
		allowed, _ := validateAPIKey(bearerToken)
		if !allowed {
			http.Error(w, "unauthorized: invalid key or rate limit exceeded", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isProtectedPath(path string) bool {
	switch path {
	case "/v1/messages", "/v1/complete", "/v1/models":
		return true
	default:
		return false
	}
}

// corsMiddleware handles CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, anthropic-version")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// maxBytesMiddleware limits request body size
func maxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip limiting for certain paths
			if r.URL.Path == "/health" || r.URL.Path == "/v1/models" {
				next.ServeHTTP(w, r)
				return
			}

			// Limit request body size (default 10MB for API requests)
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// timeoutMiddleware adds request timeout
func timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip timeout for streaming requests
			if strings.Contains(r.URL.Path, "/v1/messages") || strings.Contains(r.URL.Path, "/v1/complete") {
				next.ServeHTTP(w, r)
				return
			}

			http.TimeoutHandler(next, timeout, "request timeout").ServeHTTP(w, r)
		})
	}
}

// chainMiddleware chains multiple middleware functions
func chainMiddleware(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
