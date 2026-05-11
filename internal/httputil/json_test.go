package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorShape(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusNotFound, "Not found")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"error":"Not found"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok","extra":true}`))
	var dst struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(req, 1024, &dst); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestDecodeJSONRejectsOversizeBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"too large"}`))
	var dst struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(req, 5, &dst); err == nil {
		t.Fatal("expected body size error")
	}
}

func TestRequestIDMiddlewareSetsHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r) == "" {
			t.Fatal("missing request id in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID header")
	}
}
