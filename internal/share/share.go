package share

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

const (
	shareTokenRandomBytes = 12
	shareSlugMaxLength    = 48
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
	repo   *Repository
	client gitnote.Client
}

func NewHandler(database *db.DB, client gitnote.Client) *Handler {
	return &Handler{repo: NewRepository(database), client: client}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/shares/gitnote", h.ListGitNoteShares)
	r.Post("/api/shares/gitnote", h.CreateGitNoteShare)
	r.Patch("/api/shares/gitnote/{id}", h.UpdateGitNoteShare)
	r.Delete("/api/shares/gitnote/{id}", h.RevokeGitNoteShare)
	r.Get("/api/share/gitnote/{token}", h.GetPublicShare)
	r.Get("/api/share/gitnote/{token}/tree", h.PublicTree)
	r.Get("/api/share/gitnote/{token}/raw", h.PublicRaw)
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
	share, err := h.repo.ActiveByToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, share)
}

func (h *Handler) PublicTree(w http.ResponseWriter, r *http.Request) {
	share, err := h.repo.ActiveByToken(r.Context(), chi.URLParam(r, "token"))
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
	share, err := h.repo.ActiveByToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writePublicShareError(w, r, err)
		return
	}
	filePath, err := gitnote.NormalizePath(r.URL.Query().Get("path"))
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
		httputil.NotFound(w, r)
		return
	}
	gitnote.WriteRaw(w, file)
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

func writePublicShareError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, shared.ErrNotFound) {
		httputil.NotFound(w, r)
		return
	}
	httputil.WriteError(w, http.StatusInternalServerError, err.Error())
}
