package share

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/security"
)

func TestPublicShareRawSecurityHeadersAndFailures(t *testing.T) {
	client := &fakeGitNoteClient{
		tree: []gitnote.TreeItem{
			{Path: "notes/private/b.md", Size: 3},
			{Path: "notes/a.md", Size: 3},
		},
		raw: map[string][]byte{
			"notes/private/b.md":       []byte("# B"),
			"notes/private/page.html":  []byte("<script>alert(1)</script>"),
			"notes/private/vector.svg": []byte(`<svg onload="alert(1)"></svg>`),
			"notes/private/file.pdf":   []byte("%PDF-1.7"),
			"notes/private/image.png":  []byte{0x89, 'P', 'N', 'G'},
			"notes/private/large.md":   make([]byte, gitnote.MaxRawFileBytes+1),
		},
	}
	repo, router, cleanup := newShareTestRouter(t, client, HandlerOptions{})
	defer cleanup()

	title := "x'); DROP TABLE gitnote_shares; --"
	created, err := repo.CreateGitNoteShare(context.Background(), createRequest{PathPrefix: "notes/private", Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	token := url.PathEscape(created.Token)

	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/missing%27%20OR%201%3D1--"))

	expiredAt := "2000-01-01T00:00:00Z"
	expired, err := repo.CreateGitNoteShare(context.Background(), createRequest{PathPrefix: "notes/private", ExpiresAt: &expiredAt})
	if err != nil {
		t.Fatal(err)
	}
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+url.PathEscape(expired.Token)))

	revoked, err := repo.CreateGitNoteShare(context.Background(), createRequest{PathPrefix: "notes/private"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeGitNoteShare(context.Background(), revoked.ID); err != nil {
		t.Fatal(err)
	}
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+url.PathEscape(revoked.Token)))
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path=..%2Fsecret.md"))
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/a.md")))
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/a.md' OR 1=1 --")))

	html := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/page.html"))
	if html.Code != http.StatusOK {
		t.Fatalf("html status = %d body=%s", html.Code, html.Body.String())
	}
	if got := html.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("html Content-Type = %q", got)
	}
	if got := html.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "page.html") {
		t.Fatalf("html Content-Disposition = %q", got)
	}
	assertRawCSP(t, html)

	svg := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/vector.svg"))
	if svg.Code != http.StatusOK {
		t.Fatalf("svg status = %d body=%s", svg.Code, svg.Body.String())
	}
	if got := svg.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("svg Content-Type = %q", got)
	}
	if strings.Contains(svg.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("svg should not render as image/svg+xml")
	}
	if got := svg.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "vector.svg") {
		t.Fatalf("svg Content-Disposition = %q", got)
	}
	assertRawCSP(t, svg)

	for _, tt := range []struct {
		path        string
		contentType string
	}{
		{"notes/private/b.md", "text/plain; charset=utf-8"},
		{"notes/private/file.pdf", "application/pdf"},
		{"notes/private/image.png", "image/png"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rec := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape(tt.path))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != tt.contentType {
				t.Fatalf("Content-Type = %q", got)
			}
			if got := rec.Header().Get("Content-Disposition"); got != "" {
				t.Fatalf("Content-Disposition = %q", got)
			}
			assertRawCSP(t, rec)
		})
	}

	large := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/large.md"))
	if large.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large status = %d body=%s", large.Code, large.Body.String())
	}
	if strings.TrimSpace(large.Body.String()) != `{"error":"File too large"}` {
		t.Fatalf("large body = %q", large.Body.String())
	}
}

func TestPublicMarkdownShareAllowsReferencedMediaAndDownloadBundle(t *testing.T) {
	markdown := strings.Join([]string{
		"# B",
		"![Diagram](assets/diagram.png)",
		`<img src="../shared/photo.jpg">`,
		`<video poster="assets/poster.webp"><source src="assets/demo.mp4"></video>`,
		"![Remote](https://example.com/remote.png)",
		"![Not media](../secret.txt)",
		"![Reference][ref]",
		"[ref]: <assets/ref-image.png>",
		"![[assets/wiki-image.gif|Wiki image]]",
	}, "\n")
	client := &fakeGitNoteClient{
		tree: []gitnote.TreeItem{
			{Path: "notes/private/b.md", Size: int64(len(markdown))},
			{Path: "notes/private/assets/diagram.png", Size: 3},
			{Path: "notes/shared/photo.jpg", Size: 3},
			{Path: "notes/private/assets/poster.webp", Size: 3},
			{Path: "notes/private/assets/demo.mp4", Size: 3},
			{Path: "notes/private/assets/ref-image.png", Size: 3},
			{Path: "notes/private/assets/wiki-image.gif", Size: 3},
			{Path: "notes/private/assets/unreferenced.png", Size: 3},
			{Path: "notes/secret.txt", Size: 6},
		},
		raw: map[string][]byte{
			"notes/private/b.md":                    []byte(markdown),
			"notes/private/assets/diagram.png":      []byte("png"),
			"notes/shared/photo.jpg":                []byte("jpg"),
			"notes/private/assets/poster.webp":      []byte("webp"),
			"notes/private/assets/demo.mp4":         []byte("mp4"),
			"notes/private/assets/ref-image.png":    []byte("ref"),
			"notes/private/assets/wiki-image.gif":   []byte("gif"),
			"notes/private/assets/unreferenced.png": []byte("no"),
			"notes/secret.txt":                      []byte("secret"),
		},
	}
	repo, router, cleanup := newShareTestRouter(t, client, HandlerOptions{})
	defer cleanup()

	created, err := repo.CreateGitNoteShare(context.Background(), createRequest{PathPrefix: "notes/private/b.md"})
	if err != nil {
		t.Fatal(err)
	}
	token := url.PathEscape(created.Token)
	from := url.QueryEscape("notes/private/b.md")

	diagram := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/assets/diagram.png")+"&from="+from)
	if diagram.Code != http.StatusOK || diagram.Body.String() != "png" {
		t.Fatalf("diagram status=%d body=%s", diagram.Code, diagram.Body.String())
	}
	if got := diagram.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("diagram Content-Type = %q", got)
	}

	photo := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/shared/photo.jpg")+"&from="+from)
	if photo.Code != http.StatusOK || photo.Body.String() != "jpg" {
		t.Fatalf("photo status=%d body=%s", photo.Code, photo.Body.String())
	}

	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/assets/unreferenced.png")+"&from="+from))
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/secret.txt")+"&from="+from))
	assertShareNotFound(t, performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/assets/diagram.png")+"&from="+url.QueryEscape("notes/other.md")))

	bundle := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/download?path="+url.QueryEscape("notes/private/b.md"))
	if bundle.Code != http.StatusOK {
		t.Fatalf("bundle status = %d body=%s", bundle.Code, bundle.Body.String())
	}
	if got := bundle.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("bundle Content-Type = %q", got)
	}
	if got := bundle.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "b.zip") {
		t.Fatalf("bundle Content-Disposition = %q", got)
	}
	entries := readZipEntries(t, bundle)
	for _, name := range []string{
		"notes/private/b.md",
		"notes/private/assets/diagram.png",
		"notes/shared/photo.jpg",
		"notes/private/assets/poster.webp",
		"notes/private/assets/demo.mp4",
		"notes/private/assets/ref-image.png",
		"notes/private/assets/wiki-image.gif",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("zip missing %q; entries=%v", name, entries)
		}
	}
	if _, ok := entries["notes/private/assets/unreferenced.png"]; ok {
		t.Fatalf("zip included unreferenced image")
	}
	if _, ok := entries["notes/secret.txt"]; ok {
		t.Fatalf("zip included non-media reference")
	}
}

func TestPublicShareLimiterAppliesAcrossPublicEndpoints(t *testing.T) {
	client := &fakeGitNoteClient{
		tree: []gitnote.TreeItem{{Path: "notes/private/b.md", Size: 3}},
		raw:  map[string][]byte{"notes/private/b.md": []byte("# B")},
	}
	repo, router, cleanup := newShareTestRouter(t, client, HandlerOptions{
		PublicLimiter: security.NewRequestLimiter(2, time.Minute),
	})
	defer cleanup()

	created, err := repo.CreateGitNoteShare(context.Background(), createRequest{PathPrefix: "notes/private"})
	if err != nil {
		t.Fatal(err)
	}
	token := url.PathEscape(created.Token)

	if rec := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token); rec.Code != http.StatusOK {
		t.Fatalf("public share status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/tree"); rec.Code != http.StatusOK {
		t.Fatalf("public tree status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec := performShareRequest(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path="+url.QueryEscape("notes/private/b.md"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"Too many requests. Try again later."}` {
		t.Fatalf("rate limit body = %q", rec.Body.String())
	}
	if client.rawCalls != 0 {
		t.Fatalf("rate-limited raw request should not reach GitHub client, calls=%d", client.rawCalls)
	}
}

func newShareTestRouter(t *testing.T, client gitnote.Client, opts HandlerOptions) (*Repository, http.Handler, func()) {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: shareTestMigrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	NewHandlerWithOptions(database, client, opts).RegisterRoutes(router)
	return NewRepository(database), router, func() { _ = database.Close() }
}

func shareTestMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
}

func performShareRequest(router http.Handler, method string, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.10:12345"
	router.ServeHTTP(rec, req)
	return rec
}

func assertShareNotFound(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d want 404 body=%s", rec.Code, rec.Body.String())
	}
}

func assertRawCSP(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"sandbox", "default-src 'none'", "object-src 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("Content-Security-Policy missing %q in %q", directive, csp)
		}
	}
}

func readZipEntries(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		body, err := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = string(body)
	}
	return entries
}

type fakeGitNoteClient struct {
	tree     []gitnote.TreeItem
	raw      map[string][]byte
	rawCalls int
}

func (f *fakeGitNoteClient) Tree(ctx context.Context) ([]gitnote.TreeItem, error) {
	return f.tree, nil
}

func (f *fakeGitNoteClient) Raw(ctx context.Context, filePath string) (gitnote.RawFile, error) {
	f.rawCalls++
	normalized, err := gitnote.NormalizePath(filePath)
	if err != nil {
		return gitnote.RawFile{}, err
	}
	body, ok := f.raw[normalized]
	if !ok {
		return gitnote.RawFile{}, gitnote.HTTPError{Status: http.StatusNotFound, Message: "not found"}
	}
	return gitnote.RawFile{ContentType: gitnote.ContentType(normalized), Body: body}, nil
}
