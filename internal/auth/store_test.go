package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"private-workspace/internal/db"
)

func TestStoreBootstrapSessionLookupAndExpiry(t *testing.T) {
	ctx := context.Background()
	store, database := newTestStore(t, time.Hour)
	defer database.Close()

	hash := testPasswordHash(t, "secret")
	user, err := store.BootstrapAdmin(ctx, "Admin@Example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "admin@example.com" {
		t.Fatalf("email = %q", user.Email)
	}

	session, rawToken, err := store.CreateSession(ctx, user.ID, "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if rawToken == "" || session.ID == "" || session.CSRFSecret == "" {
		t.Fatalf("incomplete session: %#v token=%q", session, rawToken)
	}

	var storedHash string
	if err := database.QueryRowContext(ctx, "SELECT token_hash FROM sessions WHERE id = ?", session.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == rawToken {
		t.Fatal("raw session token was stored")
	}
	if storedHash != HashSessionToken(rawToken) {
		t.Fatal("stored token hash does not match token")
	}

	loaded, err := store.SessionByToken(ctx, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.User.Email != "admin@example.com" {
		t.Fatalf("loaded user email = %q", loaded.User.Email)
	}

	store.now = func() time.Time { return time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC) }
	if _, err := store.SessionByToken(ctx, rawToken); err == nil {
		t.Fatal("expected expired session to be rejected")
	}
}

func TestBootstrapAdminUpdatesHash(t *testing.T) {
	ctx := context.Background()
	store, database := newTestStore(t, time.Hour)
	defer database.Close()

	firstHash := testPasswordHash(t, "first")
	user, err := store.BootstrapAdmin(ctx, "admin@example.com", firstHash)
	if err != nil {
		t.Fatal(err)
	}
	secondHash := testPasswordHash(t, "second")
	if _, err := store.BootstrapAdmin(ctx, "admin@example.com", secondHash); err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := database.QueryRowContext(ctx, "SELECT password_hash FROM admin_users WHERE id = ?", user.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != secondHash {
		t.Fatal("admin password hash was not updated")
	}
}

func TestVerifyAdminPasswordMissingEmailUsesDummyHash(t *testing.T) {
	ctx := context.Background()
	store, database := newTestStore(t, time.Hour)
	defer database.Close()

	_, ok, err := store.VerifyAdminPassword(ctx, "missing@example.com", "wrong")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Fatal("missing user password should not verify")
	}

	original := dummyPasswordHash
	dummyPasswordHash = "malformed"
	defer func() { dummyPasswordHash = original }()

	_, _, err = store.VerifyAdminPassword(ctx, "missing@example.com", "wrong")
	if err == nil || !strings.Contains(err.Error(), "verify dummy admin password") {
		t.Fatalf("expected dummy verification error, got %v", err)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	store, database := newTestStore(t, time.Hour)
	defer database.Close()

	svc := NewService(store, Options{CookieName: "pw_session", CookieSecure: true})
	rec := httptest.NewRecorder()
	svc.setSessionCookie(rec, "token", time.Now().Add(time.Hour))

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "pw_session" || cookie.Value != "token" {
		t.Fatalf("cookie = %#v", cookie)
	}
	if !cookie.HttpOnly {
		t.Fatal("cookie should be HttpOnly")
	}
	if !cookie.Secure {
		t.Fatal("cookie should be Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("Path = %q", cookie.Path)
	}

	rec = httptest.NewRecorder()
	svc.clearSessionCookie(rec)
	if rec.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("clear cookie should set MaxAge=-1")
	}
}

func newTestStore(t *testing.T, ttl time.Duration) (*Store, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: testMigrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(database, ttl)
	store.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	return store, database
}

func testPasswordHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := hashPassword(password, Argon2Params{
		Memory:      64,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func testMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}
