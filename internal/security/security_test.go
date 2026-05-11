package security

import (
	"encoding/base64"
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
