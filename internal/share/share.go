package share

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/httputil"
	"private-workspace/internal/security"
	"private-workspace/internal/shared"
)

const (
	shareTokenRandomBytes = 12
	shareSlugMaxLength    = 48
	publicShareRateLimit  = 120
	publicShareRateWindow = time.Minute
	downloadMaxFiles      = 64
	downloadMaxBytes      = 50 << 20
)

var (
	htmlMediaTagPattern   = regexp.MustCompile(`(?is)<(?:img|video|audio|source)\b[^>]*>`)
	htmlMediaAttrPattern  = regexp.MustCompile(`(?is)\s(?:src|poster)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	markdownRefDefPattern = regexp.MustCompile(`(?m)^\s{0,3}\[([^\]\n]+)\]:\s*(<[^>\n]+>|[^\s\n]+)`)
	markdownRefUsePattern = regexp.MustCompile(`!?\[[^\]\n]*\]\[([^\]\n]+)\]`)
	wikiEmbedPattern      = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)
)

type GitNoteShare struct {
	ID         string  `json:"id"`
	PathPrefix string  `json:"pathPrefix"`
	Title      *string `json:"title"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	RevokedAt  *string `json:"revokedAt,omitempty"`
	ExpiresAt  *string `json:"expiresAt"`
}

type CreateGitNoteShareResponse struct {
	GitNoteShare
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) ListGitNoteShares(ctx context.Context) ([]GitNoteShare, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, path_prefix, title, created_at, updated_at, revoked_at, expires_at
		FROM gitnote_shares
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list gitnote shares: %w", err)
	}
	defer rows.Close()
	var shares []GitNoteShare
	for rows.Next() {
		share, err := scanGitNoteShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if shares == nil {
		shares = []GitNoteShare{}
	}
	return shares, rows.Err()
}

func (r *Repository) CreateGitNoteShare(ctx context.Context, req createRequest) (CreateGitNoteShareResponse, error) {
	pathPrefix, err := cleanPrefix(req.PathPrefix)
	if err != nil {
		return CreateGitNoteShareResponse{}, err
	}
	title := trimOptional(req.Title)
	expiresAt, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		return CreateGitNoteShareResponse{}, err
	}
	token, err := newShareToken(title, pathPrefix)
	if err != nil {
		return CreateGitNoteShareResponse{}, err
	}
	now := shared.Now()
	share := GitNoteShare{
		ID:         shared.NewID(),
		PathPrefix: pathPrefix,
		Title:      title,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  expiresAt,
	}
	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO gitnote_shares
			(id, token_hash, path_prefix, title, created_at, updated_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			share.ID,
			shared.TokenHash(token),
			share.PathPrefix,
			shared.NullString(share.Title),
			share.CreatedAt,
			share.UpdatedAt,
			shared.NullString(share.ExpiresAt),
		)
		if err != nil {
			return fmt.Errorf("create gitnote share: %w", err)
		}
		return nil
	})
	if err != nil {
		return CreateGitNoteShareResponse{}, err
	}
	return CreateGitNoteShareResponse{
		GitNoteShare: share,
		URL:          "/share/" + token,
		Token:        token,
	}, nil
}

func (r *Repository) UpdateGitNoteShare(ctx context.Context, id string, patch map[string]json.RawMessage) (GitNoteShare, error) {
	current, err := r.getGitNoteShareByID(ctx, id)
	if err != nil {
		return GitNoteShare{}, err
	}
	if raw, ok := patch["pathPrefix"]; ok {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return GitNoteShare{}, err
		}
		current.PathPrefix, err = cleanPrefix(value)
		if err != nil {
			return GitNoteShare{}, err
		}
	}
	if raw, ok := patch["title"]; ok {
		var value *string
		if string(raw) != "null" {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return GitNoteShare{}, err
			}
			value = &text
		}
		current.Title = trimOptional(value)
	}
	if raw, ok := patch["expiresAt"]; ok {
		var value *string
		if string(raw) != "null" {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return GitNoteShare{}, err
			}
			value = &text
		}
		current.ExpiresAt, err = parseExpiresAt(value)
		if err != nil {
			return GitNoteShare{}, err
		}
	}
	if raw, ok := patch["revoked"]; ok {
		var revoked bool
		if err := json.Unmarshal(raw, &revoked); err != nil {
			return GitNoteShare{}, err
		}
		if revoked {
			now := shared.Now()
			current.RevokedAt = &now
		} else {
			current.RevokedAt = nil
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE gitnote_shares
		SET path_prefix = ?, title = ?, updated_at = ?, revoked_at = ?, expires_at = ?
		WHERE id = ?`,
		current.PathPrefix,
		shared.NullString(current.Title),
		current.UpdatedAt,
		shared.NullString(current.RevokedAt),
		shared.NullString(current.ExpiresAt),
		current.ID,
	)
	if err != nil {
		return GitNoteShare{}, fmt.Errorf("update gitnote share: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return GitNoteShare{}, shared.ErrNotFound
	}
	return r.getGitNoteShareByID(ctx, id)
}

func (r *Repository) RevokeGitNoteShare(ctx context.Context, id string) error {
	now := shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE gitnote_shares
		SET revoked_at = ?, updated_at = ?
		WHERE id = ?`, now, now, id)
	if err != nil {
		return fmt.Errorf("revoke gitnote share: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func (r *Repository) ActiveByToken(ctx context.Context, token string) (GitNoteShare, error) {
	var share GitNoteShare
	var title, revokedAt, expiresAt sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, path_prefix, title, created_at, updated_at, revoked_at, expires_at
		FROM gitnote_shares
		WHERE token_hash = ?`, shared.TokenHash(strings.TrimSpace(token))).
		Scan(&share.ID, &share.PathPrefix, &title, &share.CreatedAt, &share.UpdatedAt, &revokedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GitNoteShare{}, shared.ErrNotFound
	}
	if err != nil {
		return GitNoteShare{}, fmt.Errorf("get gitnote share: %w", err)
	}
	share.Title = shared.FromNullString(title)
	share.RevokedAt = shared.FromNullString(revokedAt)
	share.ExpiresAt = shared.FromNullString(expiresAt)
	if share.RevokedAt != nil || isExpired(share.ExpiresAt) {
		return GitNoteShare{}, shared.ErrNotFound
	}
	return share, nil
}

func (r *Repository) getGitNoteShareByID(ctx context.Context, id string) (GitNoteShare, error) {
	var share GitNoteShare
	var title, revokedAt, expiresAt sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, path_prefix, title, created_at, updated_at, revoked_at, expires_at
		FROM gitnote_shares
		WHERE id = ?`, id).
		Scan(&share.ID, &share.PathPrefix, &title, &share.CreatedAt, &share.UpdatedAt, &revokedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GitNoteShare{}, shared.ErrNotFound
	}
	if err != nil {
		return GitNoteShare{}, fmt.Errorf("get gitnote share: %w", err)
	}
	share.Title = shared.FromNullString(title)
	share.RevokedAt = shared.FromNullString(revokedAt)
	share.ExpiresAt = shared.FromNullString(expiresAt)
	return share, nil
}

type Handler struct {
	repo          *Repository
	client        gitnote.Client
	publicLimiter *security.RequestLimiter
}

type HandlerOptions struct {
	PublicLimiter *security.RequestLimiter
}

func NewHandler(database *db.DB, client gitnote.Client) *Handler {
	return NewHandlerWithOptions(database, client, HandlerOptions{})
}

func NewHandlerWithOptions(database *db.DB, client gitnote.Client, opts HandlerOptions) *Handler {
	limiter := opts.PublicLimiter
	if limiter == nil {
		limiter = security.NewRequestLimiter(publicShareRateLimit, publicShareRateWindow)
	}
	return &Handler{repo: NewRepository(database), client: client, publicLimiter: limiter}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/shares/gitnote", h.ListGitNoteShares)
	r.Post("/api/shares/gitnote", h.CreateGitNoteShare)
	r.Patch("/api/shares/gitnote/{id}", h.UpdateGitNoteShare)
	r.Delete("/api/shares/gitnote/{id}", h.RevokeGitNoteShare)
	r.Get("/api/share/gitnote/{token}", h.GetPublicShare)
	r.Get("/api/share/gitnote/{token}/tree", h.PublicTree)
	r.Get("/api/share/gitnote/{token}/raw", h.PublicRaw)
	r.Get("/api/share/gitnote/{token}/download", h.PublicDownload)
}

func (h *Handler) ListGitNoteShares(w http.ResponseWriter, r *http.Request) {
	shares, err := h.repo.ListGitNoteShares(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, shares)
}

func (h *Handler) CreateGitNoteShare(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	resp, err := h.repo.CreateGitNoteShare(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) UpdateGitNoteShare(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	share, err := h.repo.UpdateGitNoteShare(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeShareError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, share)
}

func (h *Handler) RevokeGitNoteShare(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.RevokeGitNoteShare(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeShareError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) GetPublicShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.allowPublicShareRequest(w, r, token) {
		return
	}
	share, err := h.repo.ActiveByToken(r.Context(), token)
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, share)
}

func (h *Handler) PublicTree(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.allowPublicShareRequest(w, r, token) {
		return
	}
	share, err := h.repo.ActiveByToken(r.Context(), token)
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GITHUB_PAT not configured")
		return
	}
	tree, err := h.client.Tree(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string][]gitnote.TreeItem{"tree": gitnote.FilterTree(tree, share.PathPrefix)})
}

func (h *Handler) PublicRaw(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.allowPublicShareRequest(w, r, token) {
		return
	}
	share, err := h.repo.ActiveByToken(r.Context(), token)
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	filePath, err := gitnote.NormalizePath(r.URL.Query().Get("path"))
	if err != nil || !h.publicPathAllowed(r.Context(), share, filePath, r.URL.Query().Get("from")) {
		httputil.NotFound(w, r)
		return
	}
	gitnote.SetRawSecurityHeaders(w)
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GITHUB_PAT not configured")
		return
	}
	file, err := h.client.Raw(r.Context(), filePath)
	if err != nil {
		writePublicRawError(w, r, err)
		return
	}
	gitnote.WriteRaw(w, filePath, file)
}

func (h *Handler) PublicDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !h.allowPublicShareRequest(w, r, token) {
		return
	}
	share, err := h.repo.ActiveByToken(r.Context(), token)
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	filePathValue := r.URL.Query().Get("path")
	if strings.TrimSpace(filePathValue) == "" {
		filePathValue = share.PathPrefix
	}
	filePath, err := gitnote.NormalizePath(filePathValue)
	if err != nil || !gitnote.PathWithinPrefix(filePath, share.PathPrefix) {
		httputil.NotFound(w, r)
		return
	}
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GITHUB_PAT not configured")
		return
	}
	file, err := h.client.Raw(r.Context(), filePath)
	if err != nil {
		writePublicRawError(w, r, err)
		return
	}
	if !isMarkdownPath(filePath) {
		writeAttachment(w, filePath, file)
		return
	}
	if err := h.writeMarkdownBundle(w, r, filePath, file.Body); err != nil {
		writePublicRawError(w, r, err)
	}
}

func (h *Handler) publicPathAllowed(ctx context.Context, share GitNoteShare, filePath string, fromValue string) bool {
	if gitnote.PathWithinPrefix(filePath, share.PathPrefix) {
		return true
	}
	if h.client == nil || !isMediaDependencyPath(filePath) {
		return false
	}

	fromPath := share.PathPrefix
	if strings.TrimSpace(fromValue) != "" {
		normalized, err := gitnote.NormalizePath(fromValue)
		if err != nil {
			return false
		}
		fromPath = normalized
	}
	if !isMarkdownPath(fromPath) || !gitnote.PathWithinPrefix(fromPath, share.PathPrefix) {
		return false
	}
	return h.markdownReferencesPath(ctx, fromPath, filePath)
}

func (h *Handler) markdownReferencesPath(ctx context.Context, markdownPath string, targetPath string) bool {
	file, err := h.client.Raw(ctx, markdownPath)
	if err != nil {
		return false
	}
	for _, dependency := range markdownDependencyPaths(markdownPath, file.Body) {
		if dependency == targetPath {
			return true
		}
	}
	return false
}

type bundleFile struct {
	Path string
	Body []byte
}

func (h *Handler) writeMarkdownBundle(w http.ResponseWriter, r *http.Request, markdownPath string, markdownBody []byte) error {
	files := []bundleFile{{Path: markdownPath, Body: markdownBody}}
	totalBytes := len(markdownBody)
	seen := map[string]bool{markdownPath: true}

	for _, dependency := range markdownDependencyPaths(markdownPath, markdownBody) {
		if seen[dependency] {
			continue
		}
		if len(files) >= downloadMaxFiles {
			return gitnote.HTTPError{Status: http.StatusRequestEntityTooLarge, Message: "Download bundle too large"}
		}
		dependencyFile, err := h.client.Raw(r.Context(), dependency)
		if err != nil {
			var httpErr gitnote.HTTPError
			if errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound {
				continue
			}
			return err
		}
		totalBytes += len(dependencyFile.Body)
		if totalBytes > downloadMaxBytes {
			return gitnote.HTTPError{Status: http.StatusRequestEntityTooLarge, Message: "Download bundle too large"}
		}
		files = append(files, bundleFile{Path: dependency, Body: dependencyFile.Body})
		seen[dependency] = true
	}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for _, file := range files {
		header := &zip.FileHeader{
			Name:   file.Path,
			Method: zip.Deflate,
		}
		header.SetModTime(time.Now().UTC())
		entry, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = zipWriter.Close()
			return err
		}
		if _, err := entry.Write(file.Body); err != nil {
			_ = zipWriter.Close()
			return err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return err
	}

	filename := strings.TrimSuffix(path.Base(markdownPath), path.Ext(markdownPath))
	if filename == "" {
		filename = "note"
	}
	filename = safeDownloadFilename(filename + ".zip")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Cache-Control", "s-maxage=300, stale-while-revalidate=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
	return nil
}

func writeAttachment(w http.ResponseWriter, filePath string, file gitnote.RawFile) {
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": safeDownloadFilename(path.Base(filePath)),
	}))
	gitnote.WriteRaw(w, filePath, file)
}

func markdownDependencyPaths(markdownPath string, body []byte) []string {
	rawRefs := extractMarkdownMediaRefs(string(body))
	paths := make([]string, 0, len(rawRefs))
	seen := map[string]bool{}
	for _, rawRef := range rawRefs {
		resolved, ok := resolveMarkdownDependencyPath(markdownPath, rawRef)
		if !ok || !isMediaDependencyPath(resolved) || seen[resolved] {
			continue
		}
		paths = append(paths, resolved)
		seen[resolved] = true
	}
	return paths
}

func extractMarkdownMediaRefs(markdown string) []string {
	refs := extractInlineMarkdownDestinations(markdown)
	definitions := markdownReferenceDefinitions(markdown)
	for _, match := range markdownRefUsePattern.FindAllStringSubmatch(markdown, -1) {
		if len(match) <= 1 {
			continue
		}
		if ref, ok := definitions[normalizeMarkdownReferenceLabel(match[1])]; ok {
			refs = append(refs, ref)
		}
	}
	for _, match := range wikiEmbedPattern.FindAllStringSubmatch(markdown, -1) {
		if len(match) > 1 {
			ref := strings.TrimSpace(match[1])
			if idx := strings.IndexAny(ref, "|#"); idx >= 0 {
				ref = ref[:idx]
			}
			refs = append(refs, ref)
		}
	}
	for _, tag := range htmlMediaTagPattern.FindAllString(markdown, -1) {
		for _, match := range htmlMediaAttrPattern.FindAllStringSubmatch(tag, -1) {
			for i := 1; i < len(match); i++ {
				if match[i] != "" {
					refs = append(refs, match[i])
					break
				}
			}
		}
	}
	return refs
}

func markdownReferenceDefinitions(markdown string) map[string]string {
	definitions := map[string]string{}
	for _, match := range markdownRefDefPattern.FindAllStringSubmatch(markdown, -1) {
		if len(match) > 2 {
			definitions[normalizeMarkdownReferenceLabel(match[1])] = match[2]
		}
	}
	return definitions
}

func normalizeMarkdownReferenceLabel(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func extractInlineMarkdownDestinations(markdown string) []string {
	refs := []string{}
	for i := 0; i < len(markdown)-1; i++ {
		if markdown[i] != ']' || markdown[i+1] != '(' {
			continue
		}
		start := i + 2
		end := findClosingMarkdownParen(markdown, start)
		if end < 0 {
			continue
		}
		refs = append(refs, markdown[start:end])
		i = end
	}
	return refs
}

func findClosingMarkdownParen(markdown string, start int) int {
	depth := 0
	escaped := false
	for i := start; i < len(markdown); i++ {
		switch {
		case escaped:
			escaped = false
		case markdown[i] == '\\':
			escaped = true
		case markdown[i] == '(':
			depth++
		case markdown[i] == ')':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func resolveMarkdownDependencyPath(markdownPath string, rawRef string) (string, bool) {
	ref := cleanMarkdownDestination(rawRef)
	if !isLocalDependencyRef(ref) {
		return "", false
	}
	ref = stripURLSuffix(ref)
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	baseDir := path.Dir(markdownPath)
	if baseDir == "." {
		baseDir = ""
	}
	normalized, err := gitnote.NormalizePath(path.Clean(path.Join(baseDir, ref)))
	if err != nil {
		return "", false
	}
	return normalized, true
}

func cleanMarkdownDestination(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<") {
		if end := strings.IndexByte(value, '>'); end >= 0 {
			value = value[1:end]
		}
	} else {
		value = firstMarkdownDestinationToken(value)
	}
	replacer := strings.NewReplacer(`\ `, " ", `\(`, "(", `\)`, ")", `\\`, `\`)
	return strings.TrimSpace(replacer.Replace(value))
}

func firstMarkdownDestinationToken(value string) string {
	var b strings.Builder
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			break
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func isLocalDependencyRef(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || strings.Contains(value, `\`) {
		return false
	}
	if strings.HasPrefix(value, "//") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == ""
}

func stripURLSuffix(value string) string {
	if idx := strings.IndexAny(value, "?#"); idx >= 0 {
		return value[:idx]
	}
	return value
}

func isMarkdownPath(filePath string) bool {
	return strings.EqualFold(path.Ext(filePath), ".md")
}

func isMediaDependencyPath(filePath string) bool {
	switch strings.ToLower(strings.TrimPrefix(path.Ext(filePath), ".")) {
	case "png", "jpg", "jpeg", "gif", "webp", "ico", "bmp", "avif", "svg",
		"mp3", "wav", "ogg", "m4a", "flac",
		"mp4", "webm", "mov", "m4v",
		"pdf":
		return true
	default:
		return false
	}
}

func safeDownloadFilename(filePath string) string {
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

type createRequest struct {
	PathPrefix string  `json:"pathPrefix"`
	Title      *string `json:"title"`
	ExpiresAt  *string `json:"expiresAt"`
}

func cleanPrefix(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return "", errors.New("pathPrefix is required")
	}
	return gitnote.NormalizePath(value)
}

func newShareToken(title *string, pathPrefix string) (string, error) {
	label := ""
	if title != nil {
		label = *title
	}
	if strings.TrimSpace(label) == "" {
		label = lastPathSegment(pathPrefix)
	}
	if dot := strings.LastIndexByte(label, '.'); dot > 0 {
		label = label[:dot]
	}

	slug := slugifyShareLabel(label)
	if slug == "" {
		slug = "note"
	}

	suffix, err := randomURLToken(shareTokenRandomBytes)
	if err != nil {
		return "", err
	}
	return slug + "-" + suffix, nil
}

func randomURLToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func lastPathSegment(value string) string {
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	if idx := strings.LastIndexByte(value, '/'); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func slugifyShareLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false

	for _, r := range value {
		if b.Len() >= shareSlugMaxLength {
			break
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func parseExpiresAt(value *string) (*string, error) {
	value = trimOptional(value)
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, errors.New("expiresAt must be an RFC3339 timestamp")
	}
	formatted := shared.FormatTime(parsed)
	return &formatted, nil
}

func isExpired(value *string) bool {
	if value == nil {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return true
	}
	return !time.Now().UTC().Before(parsed)
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func scanGitNoteShare(scanner interface {
	Scan(dest ...any) error
}) (GitNoteShare, error) {
	var share GitNoteShare
	var title, revokedAt, expiresAt sql.NullString
	if err := scanner.Scan(&share.ID, &share.PathPrefix, &title, &share.CreatedAt, &share.UpdatedAt, &revokedAt, &expiresAt); err != nil {
		return GitNoteShare{}, fmt.Errorf("scan gitnote share: %w", err)
	}
	share.Title = shared.FromNullString(title)
	share.RevokedAt = shared.FromNullString(revokedAt)
	share.ExpiresAt = shared.FromNullString(expiresAt)
	return share, nil
}

func writeShareError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, shared.ErrNotFound) {
		httputil.NotFound(w, r)
		return
	}
	httputil.WriteError(w, http.StatusBadRequest, err.Error())
}

func (h *Handler) allowPublicShareRequest(w http.ResponseWriter, r *http.Request, token string) bool {
	if h.publicLimiter == nil {
		return true
	}
	ip := httputil.ClientIP(r)
	if h.publicLimiter.Allow("share:ip:"+ip, "share:ip-token:"+ip+":"+shared.TokenHash(token)) {
		return true
	}
	httputil.WriteError(w, http.StatusTooManyRequests, "Too many requests. Try again later.")
	return false
}

func writePublicShareError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, shared.ErrNotFound) {
		httputil.NotFound(w, r)
		return
	}
	httputil.WriteError(w, http.StatusInternalServerError, err.Error())
}

func writePublicRawError(w http.ResponseWriter, r *http.Request, err error) {
	var httpErr gitnote.HTTPError
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusRequestEntityTooLarge {
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, "File too large")
		return
	}
	httputil.NotFound(w, r)
}
