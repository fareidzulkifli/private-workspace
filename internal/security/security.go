package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func IsUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func CSRFToken(sessionID string, csrfSecret string) (string, error) {
	secret, err := base64.RawURLEncoding.DecodeString(csrfSecret)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func ValidateCSRFToken(sessionID string, csrfSecret string, token string) bool {
	expected, err := CSRFToken(sessionID, csrfSecret)
	if err != nil {
		return false
	}
	expectedBytes, err := base64.RawURLEncoding.DecodeString(expected)
	if err != nil {
		return false
	}
	actualBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return false
	}
	return hmac.Equal(expectedBytes, actualBytes)
}

type LoginLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	attempts map[string]attempt
	now      func() time.Time
}

type attempt struct {
	count      int
	windowEnds time.Time
}

func NewLoginLimiter(limit int, window time.Duration) *LoginLimiter {
	if limit <= 0 {
		limit = 5
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	return &LoginLimiter{
		limit:    limit,
		window:   window,
		attempts: make(map[string]attempt),
		now:      time.Now,
	}
}

func (l *LoginLimiter) Allow(ip string, email string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.pruneLocked(now)
	for _, key := range loginKeys(ip, email) {
		entry, ok := l.attempts[key]
		if ok && now.Before(entry.windowEnds) && entry.count >= l.limit {
			return false
		}
	}
	return true
}

func (l *LoginLimiter) RecordFailure(ip string, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for _, key := range loginKeys(ip, email) {
		entry, ok := l.attempts[key]
		if !ok || !now.Before(entry.windowEnds) {
			entry = attempt{windowEnds: now.Add(l.window)}
		}
		entry.count++
		l.attempts[key] = entry
	}
}

func (l *LoginLimiter) Reset(ip string, email string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range loginKeys(ip, email) {
		delete(l.attempts, key)
	}
}

func (l *LoginLimiter) pruneLocked(now time.Time) {
	for key, entry := range l.attempts {
		if !now.Before(entry.windowEnds) {
			delete(l.attempts, key)
		}
	}
}

func loginKeys(ip string, email string) []string {
	ip = strings.TrimSpace(ip)
	email = strings.ToLower(strings.TrimSpace(email))
	return []string{
		"ip:" + ip,
		"email:" + email,
		"ip-email:" + ip + ":" + email,
	}
}
