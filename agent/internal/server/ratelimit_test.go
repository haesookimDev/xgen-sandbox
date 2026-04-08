package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_AllowsUpToRate(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.allow("key") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_DeniesOverRate(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		rl.allow("key")
	}

	if rl.allow("key") {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiter_ResetsAfterInterval(t *testing.T) {
	rl := newRateLimiter(2, 10*time.Millisecond)

	// Exhaust tokens
	rl.allow("key")
	rl.allow("key")
	if rl.allow("key") {
		t.Error("3rd request should be denied")
	}

	// Wait for interval to pass
	time.Sleep(15 * time.Millisecond)

	if !rl.allow("key") {
		t.Error("request after interval reset should be allowed")
	}
}

func TestRateLimiter_IndependentKeys(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)

	if !rl.allow("a") {
		t.Error("first request for key 'a' should be allowed")
	}
	if rl.allow("a") {
		t.Error("second request for key 'a' should be denied")
	}

	// Key 'b' should still be allowed
	if !rl.allow("b") {
		t.Error("first request for key 'b' should be allowed")
	}
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	middleware := RateLimitMiddleware(1)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: allowed
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}

	// Second request: rate limited
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rec.Code)
	}
}

func TestRateLimitMiddleware_UsesXForwardedFor(t *testing.T) {
	middleware := RateLimitMiddleware(1)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from IP A via X-Forwarded-For
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "proxy:8080"
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req1)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}

	// Second request from same forwarded IP: should be rate limited
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req1)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request (same forwarded IP): expected 429, got %d", rec.Code)
	}

	// Request from different forwarded IP: should be allowed
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "proxy:8080"
	req2.Header.Set("X-Forwarded-For", "10.0.0.2")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req2)
	if rec.Code != http.StatusOK {
		t.Errorf("different IP request: expected 200, got %d", rec.Code)
	}
}
