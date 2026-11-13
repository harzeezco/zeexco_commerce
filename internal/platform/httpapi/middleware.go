package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type requestIDContextKey struct{}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(value)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func RequestMiddleware(next http.Handler, metrics *Metrics, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := validRequestID(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		wrapped := &statusWriter{ResponseWriter: w}
		request := r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, requestID))
		next.ServeHTTP(wrapped, request)
		if wrapped.status == 0 {
			wrapped.status = http.StatusOK
		}
		pattern := request.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		duration := time.Since(started)
		if metrics != nil {
			metrics.Observe(r.Method, pattern, wrapped.status, duration)
		}
		logger.Printf(
			"http_request request_id=%s method=%s route=%q status=%d duration_ms=%d",
			requestID, r.Method, pattern, wrapped.status, duration.Milliseconds(),
		)
	})
}

func validRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.') {
			return ""
		}
	}
	return value
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(value[:])
}

type bucket struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	buckets map[string]bucket
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, buckets: make(map[string]bucket)}
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return true, 0
	}
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.buckets[key]
	if current.windowStart.IsZero() || now.Sub(current.windowStart) >= l.window {
		current = bucket{windowStart: now}
	}
	current.lastSeen = now
	if current.count >= l.limit {
		remaining := l.window - now.Sub(current.windowStart)
		if remaining < time.Second {
			remaining = time.Second
		}
		l.buckets[key] = current
		return false, remaining
	}
	current.count++
	l.buckets[key] = current
	if len(l.buckets) > 4096 {
		for candidate, value := range l.buckets {
			if now.Sub(value.lastSeen) > 2*l.window {
				delete(l.buckets, candidate)
			}
		}
	}
	return true, 0
}

func RateLimit(limiter *Limiter, name string, metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := limiter.Allow(clientAddress(r))
		if allowed {
			next.ServeHTTP(w, r)
			return
		}
		if metrics != nil {
			metrics.IncrementRateLimited(name)
		}
		seconds := int(retryAfter.Round(time.Second).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
			"code": "RATE_LIMITED", "message": "too many requests; retry later",
		}})
	})
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) == "" {
		return "unknown"
	}
	return r.RemoteAddr
}

func RequireOrigin(allowedOrigin string, next http.Handler) http.Handler {
	allowedOrigin = strings.TrimSpace(allowedOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedOrigin == "" || r.Header.Get("Origin") != allowedOrigin {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
				"code": "CSRF_ORIGIN_REJECTED", "message": "request origin is not allowed",
			}})
			return
		}
		next.ServeHTTP(w, r)
	})
}
