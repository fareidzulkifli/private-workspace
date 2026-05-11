package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"private-workspace/internal/auth"
	"private-workspace/internal/db"
	"private-workspace/internal/security"
	"private-workspace/internal/web"
)

func TestHealthReadyAndFallbackRoutes(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	rec := perform(router, http.MethodGet, "/api/healthz", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = perform(router, http.MethodGet, "/api/readyz", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = perform(router, http.MethodGet, "/share/example", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("share fallback status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("expected html fallback, got %q", rec.Body.String())
	}
}

func TestUnknownAPIIsJSON404WhenAuthenticated(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()
	cookie, _ := login(t, router, "admin@example.com", "correct")

	rec := perform(router, http.MethodGet, "/api/missing", nil, cookie, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"Not found"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestAuthSessionLoginFailureAndRateLimit(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	rec := perform(router, http.MethodGet, "/api/auth/session", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Fatalf("session body = %s", rec.Body.String())
	}

	for i := 0; i < 5; i++ {
		rec = perform(router, http.MethodPost, "/api/auth/login", loginBody("admin@example.com", "wrong"), nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec = perform(router, http.MethodPost, "/api/auth/login", loginBody("admin@example.com", "wrong"), nil, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginSessionAndLogoutFlow(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	cookie, csrfToken := login(t, router, "admin@example.com", "correct")
	if !cookie.HttpOnly {
		t.Fatal("login cookie should be HttpOnly")
	}
	if cookie.Secure {
		t.Fatal("development test cookie should not be Secure")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v", cookie.SameSite)
	}
	if csrfToken == "" {
		t.Fatal("missing csrf token")
	}

	rec := perform(router, http.MethodGet, "/api/auth/session", nil, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Fatalf("session body = %s", rec.Body.String())
	}

	rec = perform(router, http.MethodPost, "/api/auth/logout", nil, cookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = perform(router, http.MethodPost, "/api/auth/logout", nil, cookie, csrfToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("logout body = %s", rec.Body.String())
	}

	rec = perform(router, http.MethodGet, "/api/auth/session", nil, cookie, "")
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Fatalf("session after logout body = %s", rec.Body.String())
	}
}

func TestRouteProtection(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	rec := perform(router, http.MethodGet, "/api/gitnote/tree", nil, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("private api status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = perform(router, http.MethodGet, "/dashboard", nil, nil, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("private page status = %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/login" {
		t.Fatalf("redirect location = %q", rec.Header().Get("Location"))
	}

	rec = perform(router, http.MethodGet, "/dashboard.html", nil, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nonexistent asset-like page status = %d", rec.Code)
	}

	cookie, _ := login(t, router, "admin@example.com", "correct")
	rec = perform(router, http.MethodGet, "/login", nil, cookie, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("authenticated login status = %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("redirect location = %q", rec.Header().Get("Location"))
	}
}

func TestClientRoutesWithFileExtensionsServeAppShell(t *testing.T) {
	router, cleanup := newTestRouter(t)
	defer cleanup()

	rec := perform(router, http.MethodGet, "/gitnote/SQL%20Scripts/Luckyfrozen%202.md", nil, nil, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("unauthenticated gitnote file route status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated gitnote file route redirect = %q", rec.Header().Get("Location"))
	}

	cookie, _ := login(t, router, "admin@example.com", "correct")
	rec = perform(router, http.MethodGet, "/gitnote/SQL%20Scripts/Luckyfrozen%202.md", nil, cookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated gitnote file route status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("expected gitnote app shell, got %q", rec.Body.String())
	}

	rec = perform(router, http.MethodGet, "/share/token/SQL%20Scripts/Luckyfrozen%202.md", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("share file route status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("expected share app shell, got %q", rec.Body.String())
	}
}

func TestPublicClassification(t *testing.T) {
	cases := []struct {
		method string
		path   string
		public bool
	}{
		{http.MethodGet, "/login", true},
		{http.MethodGet, "/share/note", true},
		{http.MethodGet, "/api/healthz", true},
		{http.MethodGet, "/api/auth/session", true},
		{http.MethodPost, "/api/auth/login", true},
		{http.MethodGet, "/api/gitnote/tree", false},
		{http.MethodPost, "/api/auth/logout", false},
		{http.MethodGet, "/dashboard", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := IsPublicRequest(req); got != tc.public {
			t.Fatalf("%s %s public=%v, want %v", tc.method, tc.path, got, tc.public)
		}
	}
}

func TestConcreteStaticAssetIsPublic(t *testing.T) {
	publicDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(publicDir, "file.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	router, cleanup := newTestRouterWithWeb(t, web.New(web.Options{
		DistDir:   filepath.Join(t.TempDir(), "dist"),
		PublicDir: publicDir,
		Favicon:   filepath.Join(t.TempDir(), "favicon.ico"),
	}))
	defer cleanup()

	rec := perform(router, http.MethodGet, "/file.svg", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q", got)
	}

	rec = perform(router, http.MethodGet, "/assets/index-stalehash.js", nil, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("missing asset content type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestStaleIndexAssetsRedirectToCurrentBuild(t *testing.T) {
	distDir := t.TempDir()
	assetsDir := filepath.Join(distDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := `<!doctype html>
<html>
<head>
  <script type="module" crossorigin src="/assets/index-DkVOd-0Q.js"></script>
  <link rel="stylesheet" crossorigin href="/assets/index-HqAuOnPj.css">
</head>
<body><div id="root"></div></body>
</html>`
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index-DkVOd-0Q.js"), []byte("console.log('current')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index-HqAuOnPj.css"), []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}

	router, cleanup := newTestRouterWithWeb(t, web.New(web.Options{
		DistDir:   distDir,
		PublicDir: filepath.Join(t.TempDir(), "public"),
		Favicon:   filepath.Join(t.TempDir(), "favicon.ico"),
	}))
	defer cleanup()

	rec := perform(router, http.MethodGet, "/assets/index-B8P8_NAv.css", nil, nil, "")
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("stale css status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/assets/index-HqAuOnPj.css" {
		t.Fatalf("stale css redirect = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("stale css Cache-Control = %q", got)
	}

	rec = perform(router, http.MethodGet, "/assets/index-BED9f0ve.js", nil, nil, "")
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("stale js status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/assets/index-DkVOd-0Q.js" {
		t.Fatalf("stale js redirect = %q", got)
	}

	rec = perform(router, http.MethodGet, "/assets/index-DkVOd-0Q.js", nil, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("current js status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("current js Cache-Control = %q", got)
	}
}

func newTestRouter(t *testing.T) (http.Handler, func()) {
	t.Helper()
	emptyDir := t.TempDir()
	return newTestRouterWithWeb(t, web.New(web.Options{
		DistDir:   filepath.Join(emptyDir, "dist"),
		PublicDir: filepath.Join(emptyDir, "public"),
		Favicon:   filepath.Join(emptyDir, "favicon.ico"),
	}))
}

func newTestRouterWithWeb(t *testing.T, webHandler *web.Handler) (http.Handler, func()) {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: testMigrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}

	store := auth.NewStore(database, time.Hour)
	if _, err := store.BootstrapAdmin(context.Background(), "admin@example.com", testAdminHash(t)); err != nil {
		t.Fatal(err)
	}

	authService := auth.NewService(store, auth.Options{
		CookieName:     "pw_session",
		CookieSecure:   false,
		CSRFHeaderName: "X-CSRF-Token",
		Limiter:        security.NewLoginLimiter(5, 15*time.Minute),
	})

	router := NewRouter(Config{
		DB:   database,
		Auth: authService,
		Web:  webHandler,
	})
	return router, func() { _ = database.Close() }
}

func login(t *testing.T, router http.Handler, email string, password string) (*http.Cookie, string) {
	t.Helper()
	rec := perform(router, http.MethodPost, "/api/auth/login", loginBody(email, password), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return cookies[0], body.CSRFToken
}

func perform(router http.Handler, method string, path string, body []byte, cookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func loginBody(email string, password string) []byte {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	return body
}

var (
	adminHashOnce sync.Once
	adminHash     string
	adminHashErr  error
)

func testAdminHash(t *testing.T) string {
	t.Helper()
	adminHashOnce.Do(func() {
		adminHash, adminHashErr = auth.HashPassword("correct")
	})
	if adminHashErr != nil {
		t.Fatal(adminHashErr)
	}
	return adminHash
}

func testMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}
