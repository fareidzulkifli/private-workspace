package security

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCSRFTokenValidation(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	token, err := CSRFToken("session-id", secret)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateCSRFToken("session-id", secret, token) {
		t.Fatal("expected csrf token to validate")
	}
	if ValidateCSRFToken("other-session", secret, token) {
		t.Fatal("expected csrf token to reject other session")
	}
	if ValidateCSRFToken("session-id", secret, "bad-token") {
		t.Fatal("expected malformed csrf token to reject")
	}
}

func TestLoginLimiterAllowBlockAndReset(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewLoginLimiter(5, 15*time.Minute)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if !limiter.Allow("127.0.0.1", "admin@example.com") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		limiter.RecordFailure("127.0.0.1", "admin@example.com")
	}
	if limiter.Allow("127.0.0.1", "admin@example.com") {
		t.Fatal("sixth failed attempt should be blocked")
	}

	limiter.Reset("127.0.0.1", "admin@example.com")
	if !limiter.Allow("127.0.0.1", "admin@example.com") {
		t.Fatal("reset should allow login")
	}

	limiter.RecordFailure("127.0.0.1", "admin@example.com")
	now = now.Add(16 * time.Minute)
	if !limiter.Allow("127.0.0.1", "admin@example.com") {
		t.Fatal("expired window should allow login")
	}
}

func TestRequestLimiterBlocksOnAnyKeyAndResetsAfterWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := NewRequestLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }

	if !limiter.Allow("ip:127.0.0.1", "ip-token:127.0.0.1:a") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("ip:127.0.0.1", "ip-token:127.0.0.1:a") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("ip:127.0.0.1", "ip-token:127.0.0.1:b") {
		t.Fatal("shared IP key should block even when token key changes")
	}

	now = now.Add(time.Minute + time.Second)
	if !limiter.Allow("ip:127.0.0.1", "ip-token:127.0.0.1:b") {
		t.Fatal("expired window should allow requests")
	}
}

func TestHeadersSetsBrowserSecurityHeaders(t *testing.T) {
	handler := Headers(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := rec.Header().Get("Permissions-Policy"); got != "camera=(), microphone=(), geolocation=()" {
		t.Fatalf("Permissions-Policy = %q", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("Content-Security-Policy missing %q in %q", directive, csp)
		}
	}
}
