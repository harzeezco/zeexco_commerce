package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimitRejectsAndSetsRetryAfter(t *testing.T) {
	limiter := NewLimiter(1, time.Minute)
	metrics := NewMetrics()
	handler := RateLimit(limiter, "login", metrics, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.RemoteAddr = "203.0.113.10:1234"
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, request)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status=%d", first.Code)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request)
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" ||
		!strings.Contains(second.Body.String(), "RATE_LIMITED") {
		t.Fatalf("second status=%d headers=%v body=%q", second.Code, second.Header(), second.Body.String())
	}
}

func TestRequireOriginFailsClosed(t *testing.T) {
	handler := RequireOrigin("https://app.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, origin := range []string{"", "https://attacker.example"} {
		request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("origin=%q status=%d", origin, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("allowed status=%d", response.Code)
	}
}

func TestRequestMiddlewarePropagatesRequestIDAndRecordsMetrics(t *testing.T) {
	metrics := NewMetrics()
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r.Context()) != "client-request-1" {
			t.Fatalf("request ID=%q", RequestID(r.Context()))
		}
		w.WriteHeader(http.StatusCreated)
	})
	handler := RequestMiddleware(mux, metrics, log.New(&logs, "", 0))
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set("X-Request-ID", "client-request-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != "client-request-1" ||
		!strings.Contains(logs.String(), "request_id=client-request-1") ||
		!strings.Contains(metrics.render(), `route="GET /test"`) {
		t.Fatalf("headers=%v logs=%q metrics=%q", response.Header(), logs.String(), metrics.render())
	}
}

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return errors.New("unavailable") }

func TestHealthHandlers(t *testing.T) {
	live := httptest.NewRecorder()
	Live(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("live status=%d", live.Code)
	}
	ready := httptest.NewRecorder()
	Ready(failingPinger{})(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status=%d", ready.Code)
	}
}
