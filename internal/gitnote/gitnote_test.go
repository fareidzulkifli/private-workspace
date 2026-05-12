package gitnote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteRawActiveBrowserFormatsAreInertDownloads(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"notes/page.html", "text/html; charset=utf-8"},
		{"notes/vector.svg", "image/svg+xml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteRaw(rec, tt.path, RawFile{ContentType: tt.contentType, Body: []byte("<script>alert(1)</script>")})

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
			disposition := rec.Header().Get("Content-Disposition")
			if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, safeAttachmentFilename(tt.path)) {
				t.Fatalf("Content-Disposition = %q", disposition)
			}
			if got := rec.Header().Get("Content-Security-Policy"); got != rawCSP {
				t.Fatalf("Content-Security-Policy = %q", got)
			}
		})
	}
}

func TestWriteRawUsableFormatsKeepContentTypesAndRawCSP(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"notes/readme.md", "text/plain; charset=utf-8"},
		{"notes/report.pdf", "application/pdf"},
		{"notes/image.png", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteRaw(rec, tt.path, RawFile{ContentType: tt.contentType, Body: []byte("ok")})

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != tt.contentType {
				t.Fatalf("Content-Type = %q", got)
			}
			if got := rec.Header().Get("Content-Disposition"); got != "" {
				t.Fatalf("Content-Disposition = %q", got)
			}
			if got := rec.Header().Get("Content-Security-Policy"); got != rawCSP {
				t.Fatalf("Content-Security-Policy = %q", got)
			}
		})
	}
}

func TestWriteRawOversizedBodyReturns413(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteRaw(rec, "notes/large.md", RawFile{ContentType: "text/plain; charset=utf-8", Body: make([]byte, MaxRawFileBytes+1)})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"File too large"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGitHubClientRawRejectsContentLengthOverLimit(t *testing.T) {
	client := testGitHubClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: MaxRawFileBytes + 1,
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})

	_, err := client.Raw(context.Background(), "notes/large.md")
	assertFileTooLarge(t, err)
}

func TestGitHubClientRawRejectsUnknownLengthOverLimit(t *testing.T) {
	client := testGitHubClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: -1,
			Body:          io.NopCloser(bytes.NewReader(make([]byte, MaxRawFileBytes+1))),
		}, nil
	})

	_, err := client.Raw(context.Background(), "notes/large.md")
	assertFileTooLarge(t, err)
}

func assertFileTooLarge(t *testing.T, err error) {
	t.Helper()
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T %v", err, err)
	}
	if httpErr.Status != http.StatusRequestEntityTooLarge || httpErr.Message != "File too large" {
		t.Fatalf("HTTPError = %#v", httpErr)
	}
}

func testGitHubClient(fn func(*http.Request) (*http.Response, error)) *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{Transport: roundTripFunc(fn)},
		token:      "token",
		owner:      "owner",
		repo:       "repo",
		branch:     "main",
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
