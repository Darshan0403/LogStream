// internal/api/middleware.go
package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// APIKeyAuth validates the X-API-Key header. Returns 401 if missing or invalid.
func APIKeyAuth(validKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-Key") != validKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder is a custom ResponseWriter to capture the status code for logging
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// FIX: Allow gorilla/websocket to hijack the underlying TCP connection
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

// FIX: Pass through Flush for streams
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Logger logs the method, path, status code, and execution duration
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		fmt.Printf("%s %s → %d (%v)\n", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// CORS handles Cross-Origin Resource Sharing and preflight OPTIONS requests
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "X-API-Key, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recoverer catches panics, logs the stack trace, and safely returns a 500 error
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("CRITICAL PANIC: %v\n%s\n", err, debug.Stack())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Global map to hold a rate limiter for every single IP address that visits us
var visitors = make(map[string]*rate.Limiter)

// RateLimit middleware protects endpoints from spam using a token bucket
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the IP address (stripping the port)
		ip := r.RemoteAddr
		if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
			ip = ip[:colonIdx]
		}

		// TRAP 1: Missing sync.Mutex.
		// If two new IPs hit this exact block of code at the exact same millisecond,
		// Go will throw a fatal "concurrent map writes" panic and crash the server.
		//
		// TRAP 2: Memory Leak.
		// We are adding IPs to this map forever. We never clean up old IPs.
		// If this is on the public internet, a botnet scanning ports will fill
		// our RAM with millions of rate limiters until the server OOM crashes.

		limiter, exists := visitors[ip]
		if !exists {
			// Allow 20 requests per second, with a maximum burst of 50
			limiter = rate.NewLimiter(20, 50)
			visitors[ip] = limiter
		}

		if !limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limit exceeded - slow down"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
