package gitnote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/httputil"
)

type TreeItem struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type RawFile struct {
	ContentType string
	Body        []byte
}

const (
	MaxRawFileBytes = 10 << 20
	rawCSP          = "sandbox; default-src 'none'; img-src 'self' data: blob:; style-src 'unsafe-inline'; media-src 'self' blob:; object-src 'none'; base-uri 'none'; form-action 'none'"
)

type Client interface {
	Tree(ctx context.Context) ([]TreeItem, error)
	Raw(ctx context.Context, filePath string) (RawFile, error)
}

type GitHubClient struct {
	httpClient *http.Client
	token      string
	owner      string
	repo       string
	branch     string
}

type GitHubConfig struct {
	Token  string
	Owner  string
	Repo   string
	Branch string
}

func NewGitHubClient(cfg GitHubConfig) *GitHubClient {
	if cfg.Owner == "" {
		cfg.Owner = "fareidzulkifli"
	}
	if cfg.Repo == "" {
		cfg.Repo = "BA-notes"
	}
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	return &GitHubClient{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		token:      strings.TrimSpace(cfg.Token),
		owner:      strings.TrimSpace(cfg.Owner),
		repo:       strings.TrimSpace(cfg.Repo),
		branch:     strings.TrimSpace(cfg.Branch),
	}
}

func (c *GitHubClient) Tree(ctx context.Context) ([]TreeItem, error) {
	if c == nil || c.token == "" {
		return nil, errors.New("GITHUB_PAT not configured")
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/HEAD?recursive=1", url.PathEscape(c.owner), url.PathEscape(c.repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub tree API: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil, HTTPError{Status: res.StatusCode, Message: fmt.Sprintf("GitHub API error: %d", res.StatusCode)}
	}

	var payload struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode GitHub tree: %w", err)
	}
	tree := make([]TreeItem, 0, len(payload.Tree))
	for _, item := range payload.Tree {
		if item.Type == "blob" {
			tree = append(tree, TreeItem{Path: item.Path, Size: item.Size})
		}
	}
	if tree == nil {
		tree = []TreeItem{}
	}
	return tree, nil
}

func (c *GitHubClient) Raw(ctx context.Context, filePath string) (RawFile, error) {
	if c == nil || c.token == "" {
		return RawFile{}, errors.New("GITHUB_PAT not configured")
	}
	normalized, err := NormalizePath(filePath)
	if err != nil {
		return RawFile{}, err
	}
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		url.PathEscape(c.owner),
		url.PathEscape(c.repo),
		url.PathEscape(c.branch),
		escapePath(normalized),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return RawFile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return RawFile{}, fmt.Errorf("GitHub raw fetch: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, res.Body)
		return RawFile{}, HTTPError{Status: res.StatusCode, Message: fmt.Sprintf("File not found (%d)", res.StatusCode)}
	}
	if res.ContentLength > MaxRawFileBytes {
		return RawFile{}, HTTPError{Status: http.StatusRequestEntityTooLarge, Message: "File too large"}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxRawFileBytes+1))
	if err != nil {
		return RawFile{}, fmt.Errorf("read GitHub raw body: %w", err)
	}
	if int64(len(body)) > MaxRawFileBytes {
		return RawFile{}, HTTPError{Status: http.StatusRequestEntityTooLarge, Message: "File too large"}
	}
	return RawFile{ContentType: ContentType(normalized), Body: body}, nil
}

type HTTPError struct {
	Status  int
	Message string
}

func (e HTTPError) Error() string {
	return e.Message
}

type Handler struct {
	client Client
}

func NewHandler(client Client) *Handler {
	return &Handler{client: client}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/gitnote/tree", h.Tree)
	r.Get("/api/gitnote/raw", h.Raw)
}

func (h *Handler) Tree(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GITHUB_PAT not configured")
		return
	}
	tree, err := h.client.Tree(r.Context())
	if err != nil {
		writeGitNoteError(w, err)
		return
	}
	if tree == nil {
		tree = []TreeItem{}
	}
	w.Header().Set("Cache-Control", "s-maxage=300, stale-while-revalidate=60")
	httputil.WriteJSON(w, http.StatusOK, map[string][]TreeItem{"tree": tree})
}

func (h *Handler) Raw(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GITHUB_PAT not configured")
		return
	}
	filePath := r.URL.Query().Get("path")
	normalized, err := NormalizePath(filePath)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	SetRawSecurityHeaders(w)
	file, err := h.client.Raw(r.Context(), normalized)
	if err != nil {
		writeGitNoteError(w, err)
		return
	}
	WriteRaw(w, normalized, file)
}

func WriteRaw(w http.ResponseWriter, filePath string, file RawFile) {
	normalized, err := NormalizePath(filePath)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	SetRawSecurityHeaders(w)
	if int64(len(file.Body)) > MaxRawFileBytes {
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, "File too large")
		return
	}
	contentType := file.ContentType
	if strings.TrimSpace(contentType) == "" {
		contentType = ContentType(normalized)
	}
	if IsActiveBrowserFormat(normalized) {
		contentType = "text/plain; charset=utf-8"
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": safeAttachmentFilename(normalized),
		}))
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "s-maxage=300, stale-while-revalidate=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Body)
}

func SetRawSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", rawCSP)
}

func NormalizePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return "", errors.New("invalid path")
	}
	decoded := value
	for i := 0; i < 3; i++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return "", err
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	if strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "\\") {
		return "", errors.New("invalid path")
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("invalid path")
		}
	}
	cleaned := path.Clean(decoded)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") || cleaned != decoded {
		return "", errors.New("invalid path")
	}
	return cleaned, nil
}

func PathWithinPrefix(filePath string, prefix string) bool {
	if filePath == prefix {
		return true
	}
	return strings.HasPrefix(filePath, strings.TrimSuffix(prefix, "/")+"/")
}

func FilterTree(tree []TreeItem, prefix string) []TreeItem {
	filtered := make([]TreeItem, 0, len(tree))
	for _, item := range tree {
		if PathWithinPrefix(item.Path, prefix) {
			filtered = append(filtered, item)
		}
	}
	if filtered == nil {
		filtered = []TreeItem{}
	}
	return filtered
}

func ContentType(filePath string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filePath), "."))
	switch ext {
	case "pdf":
		return "application/pdf"
	case "md", "txt", "js", "ts", "py":
		return "text/plain; charset=utf-8"
	case "json":
		return "application/json; charset=utf-8"
	case "html":
		return "text/html; charset=utf-8"
	case "css":
		return "text/css; charset=utf-8"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "doc":
		return "application/msword"
	case "xlsx", "xlxs":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "xls":
		return "application/vnd.ms-excel"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "ico":
		return "image/x-icon"
	default:
		if typ := mime.TypeByExtension("." + ext); typ != "" {
			return typ
		}
		return "application/octet-stream"
	}
}

func IsActiveBrowserFormat(filePath string) bool {
	switch strings.ToLower(strings.TrimPrefix(path.Ext(filePath), ".")) {
	case "html", "htm", "xhtml", "svg", "xml":
		return true
	default:
		return false
	}
}

func writeGitNoteError(w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		httputil.WriteError(w, httpErr.Status, httpErr.Message)
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		httputil.WriteError(w, http.StatusGatewayTimeout, err.Error())
		return
	}
	httputil.WriteError(w, http.StatusInternalServerError, err.Error())
}

func safeAttachmentFilename(filePath string) string {
	name := path.Base(filePath)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
