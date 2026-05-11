package prompts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Template struct {
	ID          string   `json:"id"`
	OrgID       *string  `json:"org_id"`
	Title       string   `json:"title"`
	Description *string  `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Body        string   `json:"body"`
	IsFavorite  bool     `json:"is_favorite"`
	ArchivedAt  *string  `json:"archived_at"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type ContextPack struct {
	ID          string            `json:"id"`
	OrgID       *string           `json:"org_id"`
	Title       string            `json:"title"`
	Description *string           `json:"description"`
	Tags        []string          `json:"tags"`
	ArchivedAt  *string           `json:"archived_at"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	Items       []ContextPackItem `json:"items"`
}

type ContextPackItem struct {
	ID               string  `json:"id"`
	ContextPackID    string  `json:"context_pack_id"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	SortOrder        float64 `json:"sort_order"`
	EnabledByDefault bool    `json:"enabled_by_default"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) ListTemplates(ctx context.Context, includeArchived bool) ([]Template, error) {
	where := ""
	if !includeArchived {
		where = "WHERE archived_at IS NULL"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, org_id, title, description, category, tags, body,
			is_favorite, archived_at, created_at, updated_at
		FROM prompt_templates `+where+`
		ORDER BY is_favorite DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list prompt templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if templates == nil {
		templates = []Template{}
	}
	return templates, rows.Err()
}

func (r *Repository) CreateTemplate(ctx context.Context, req templateInput) (Template, error) {
	template, err := sanitizeTemplate(req, false)
	if err != nil {
		return Template{}, err
	}
	now := shared.Now()
	template.ID = shared.NewID()
	template.CreatedAt = now
	template.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `INSERT INTO prompt_templates
		(id, org_id, title, description, category, tags, body, is_favorite, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		template.ID,
		shared.NullString(template.OrgID),
		template.Title,
		shared.NullString(template.Description),
		template.Category,
		shared.JSONTags(template.Tags),
		template.Body,
		shared.BoolInt(template.IsFavorite),
		shared.NullString(template.ArchivedAt),
		template.CreatedAt,
		template.UpdatedAt,
	)
	if err != nil {
		return Template{}, fmt.Errorf("create prompt template: %w", err)
	}
	return r.GetTemplate(ctx, template.ID)
}

func (r *Repository) GetTemplate(ctx context.Context, id string) (Template, error) {
	var template Template
	var orgID, description, archivedAt sql.NullString
	var tags string
	var favorite int
	err := r.db.QueryRowContext(ctx, `SELECT id, org_id, title, description, category, tags, body,
			is_favorite, archived_at, created_at, updated_at
		FROM prompt_templates WHERE id = ?`, id).
		Scan(&template.ID, &orgID, &template.Title, &description, &template.Category, &tags, &template.Body, &favorite, &archivedAt, &template.CreatedAt, &template.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, shared.ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("get prompt template: %w", err)
	}
	template.OrgID = shared.FromNullString(orgID)
	template.Description = shared.FromNullString(description)
	template.Tags = shared.ParseTags(tags)
	template.IsFavorite = shared.IntBool(favorite)
	template.ArchivedAt = shared.FromNullString(archivedAt)
	return template, nil
}

func (r *Repository) UpdateTemplate(ctx context.Context, id string, patch map[string]json.RawMessage) (Template, error) {
	current, err := r.GetTemplate(ctx, id)
	if err != nil {
		return Template{}, err
	}
	if err := applyTemplatePatch(&current, patch); err != nil {
		return Template{}, err
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE prompt_templates
		SET org_id = ?, title = ?, description = ?, category = ?, tags = ?, body = ?,
			is_favorite = ?, archived_at = ?, updated_at = ?
		WHERE id = ?`,
		shared.NullString(current.OrgID),
		current.Title,
		shared.NullString(current.Description),
		current.Category,
		shared.JSONTags(current.Tags),
		current.Body,
		shared.BoolInt(current.IsFavorite),
		shared.NullString(current.ArchivedAt),
		current.UpdatedAt,
		id,
	)
	if err != nil {
		return Template{}, fmt.Errorf("update prompt template: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Template{}, shared.ErrNotFound
	}
	return r.GetTemplate(ctx, id)
}

func (r *Repository) ListContextPacks(ctx context.Context, includeArchived bool) ([]ContextPack, error) {
	where := ""
	if !includeArchived {
		where = "WHERE archived_at IS NULL"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, org_id, title, description, tags, archived_at, created_at, updated_at
		FROM context_packs `+where+`
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list context packs: %w", err)
	}
	defer rows.Close()

	var packs []ContextPack
	for rows.Next() {
		pack, err := scanContextPack(rows)
		if err != nil {
			return nil, err
		}
		packs = append(packs, pack)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if packs == nil {
		packs = []ContextPack{}
	}
	return r.attachItems(ctx, packs)
}

func (r *Repository) CreateContextPack(ctx context.Context, req packInput) (ContextPack, error) {
	var id string
	err := r.db.Tx(ctx, func(tx *sql.Tx) error {
		pack, err := sanitizePack(req, false)
		if err != nil {
			return err
		}
		now := shared.Now()
		id = shared.NewID()
		pack.ID = id
		pack.CreatedAt = now
		pack.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `INSERT INTO context_packs
			(id, org_id, title, description, tags, archived_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			pack.ID,
			shared.NullString(pack.OrgID),
			pack.Title,
			shared.NullString(pack.Description),
			shared.JSONTags(pack.Tags),
			shared.NullString(pack.ArchivedAt),
			pack.CreatedAt,
			pack.UpdatedAt,
		); err != nil {
			return fmt.Errorf("create context pack: %w", err)
		}
		items := sanitizeItems(req.Items)
		for _, item := range items {
			if err := insertContextPackItem(ctx, tx, id, item, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ContextPack{}, err
	}
	return r.GetContextPack(ctx, id)
}

func (r *Repository) GetContextPack(ctx context.Context, id string) (ContextPack, error) {
	var pack ContextPack
	var orgID, description, archivedAt sql.NullString
	var tags string
	err := r.db.QueryRowContext(ctx, `SELECT id, org_id, title, description, tags, archived_at, created_at, updated_at
		FROM context_packs WHERE id = ?`, id).
		Scan(&pack.ID, &orgID, &pack.Title, &description, &tags, &archivedAt, &pack.CreatedAt, &pack.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ContextPack{}, shared.ErrNotFound
	}
	if err != nil {
		return ContextPack{}, fmt.Errorf("get context pack: %w", err)
	}
	pack.OrgID = shared.FromNullString(orgID)
	pack.Description = shared.FromNullString(description)
	pack.Tags = shared.ParseTags(tags)
	pack.ArchivedAt = shared.FromNullString(archivedAt)
	items, err := listContextPackItems(ctx, r.db, id)
	if err != nil {
		return ContextPack{}, err
	}
	pack.Items = items
	return pack, nil
}

func (r *Repository) UpdateContextPack(ctx context.Context, id string, patch map[string]json.RawMessage) (ContextPack, error) {
	err := r.db.Tx(ctx, func(tx *sql.Tx) error {
		current, err := getContextPackOnly(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := applyPackPatch(&current, patch); err != nil {
			return err
		}
		current.UpdatedAt = shared.Now()
		result, err := tx.ExecContext(ctx, `UPDATE context_packs
			SET org_id = ?, title = ?, description = ?, tags = ?, archived_at = ?, updated_at = ?
			WHERE id = ?`,
			shared.NullString(current.OrgID),
			current.Title,
			shared.NullString(current.Description),
			shared.JSONTags(current.Tags),
			shared.NullString(current.ArchivedAt),
			current.UpdatedAt,
			id,
		)
		if err != nil {
			return fmt.Errorf("update context pack: %w", err)
		}
		if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
			return shared.ErrNotFound
		}
		if raw, ok := patch["items"]; ok {
			var reqItems []packItemInput
			if string(raw) != "null" {
				if err := json.Unmarshal(raw, &reqItems); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM context_pack_items WHERE context_pack_id = ?", id); err != nil {
				return fmt.Errorf("replace context pack items: %w", err)
			}
			items := sanitizeItems(reqItems)
			for _, item := range items {
				if err := insertContextPackItem(ctx, tx, id, item, current.UpdatedAt); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return ContextPack{}, err
	}
	return r.GetContextPack(ctx, id)
}

func (r *Repository) attachItems(ctx context.Context, packs []ContextPack) ([]ContextPack, error) {
	if len(packs) == 0 {
		return []ContextPack{}, nil
	}
	for i := range packs {
		items, err := listContextPackItems(ctx, r.db, packs[i].ID)
		if err != nil {
			return nil, err
		}
		packs[i].Items = items
	}
	return packs, nil
}

type Handler struct {
	repo *Repository
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{repo: NewRepository(database)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/prompts/templates", h.ListTemplates)
	r.Post("/api/prompts/templates", h.CreateTemplate)
	r.Get("/api/prompts/templates/{id}", h.GetTemplate)
	r.Patch("/api/prompts/templates/{id}", h.UpdateTemplate)
	r.Get("/api/prompts/context-packs", h.ListContextPacks)
	r.Post("/api/prompts/context-packs", h.CreateContextPack)
	r.Get("/api/prompts/context-packs/{id}", h.GetContextPack)
	r.Patch("/api/prompts/context-packs/{id}", h.UpdateContextPack)
}

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.repo.ListTemplates(r.Context(), shared.ParseBoolQuery(r, "include_archived"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, templates)
}

func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req templateInput
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	template, err := h.repo.CreateTemplate(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, template)
}

func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	template, err := h.repo.GetTemplate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, template)
}

func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	template, err := h.repo.UpdateTemplate(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeBadRequestOrNotFound(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, template)
}

func (h *Handler) ListContextPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := h.repo.ListContextPacks(r.Context(), shared.ParseBoolQuery(r, "include_archived"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, packs)
}

func (h *Handler) CreateContextPack(w http.ResponseWriter, r *http.Request) {
	var req packInput
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	pack, err := h.repo.CreateContextPack(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, pack)
}

func (h *Handler) GetContextPack(w http.ResponseWriter, r *http.Request) {
	pack, err := h.repo.GetContextPack(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pack)
}

func (h *Handler) UpdateContextPack(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	pack, err := h.repo.UpdateContextPack(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeBadRequestOrNotFound(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, pack)
}

type templateInput struct {
	OrgID       *string  `json:"org_id"`
	Title       string   `json:"title"`
	Description *string  `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Body        string   `json:"body"`
	IsFavorite  bool     `json:"is_favorite"`
	Archived    bool     `json:"archived"`
}

type packInput struct {
	OrgID       *string         `json:"org_id"`
	Title       string          `json:"title"`
	Description *string         `json:"description"`
	Tags        []string        `json:"tags"`
	Archived    bool            `json:"archived"`
	Items       []packItemInput `json:"items"`
}

type packItemInput struct {
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	SortOrder        float64 `json:"sort_order"`
	EnabledByDefault *bool   `json:"enabled_by_default"`
}

func sanitizeTemplate(req templateInput, partial bool) (Template, error) {
	title := strings.TrimSpace(req.Title)
	if !partial && title == "" {
		return Template{}, errors.New("Prompt title is required")
	}
	body := strings.TrimSpace(req.Body)
	if !partial && body == "" {
		return Template{}, errors.New("Prompt body is required")
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "General"
	}
	template := Template{
		OrgID:       trimOptional(req.OrgID),
		Title:       title,
		Description: trimOptional(req.Description),
		Category:    category,
		Tags:        shared.NormalizeTags(req.Tags),
		Body:        body,
		IsFavorite:  req.IsFavorite,
	}
	if req.Archived {
		now := shared.Now()
		template.ArchivedAt = &now
	}
	return template, nil
}

func applyTemplatePatch(template *Template, patch map[string]json.RawMessage) error {
	var err error
	if raw, ok := patch["org_id"]; ok {
		template.OrgID, err = shared.ParseOptionalString(raw)
		if err != nil {
			return err
		}
		template.OrgID = trimOptional(template.OrgID)
	}
	if raw, ok := patch["title"]; ok {
		template.Title, err = shared.ParseRequiredString(raw)
		if err != nil || template.Title == "" {
			return errors.New("Prompt title is required")
		}
	}
	if raw, ok := patch["body"]; ok {
		template.Body, err = shared.ParseRequiredString(raw)
		if err != nil || template.Body == "" {
			return errors.New("Prompt body is required")
		}
	}
	if raw, ok := patch["description"]; ok {
		template.Description, err = shared.ParseOptionalString(raw)
		if err != nil {
			return err
		}
		template.Description = trimOptional(template.Description)
	}
	if raw, ok := patch["category"]; ok {
		template.Category, err = shared.ParseRequiredString(raw)
		if err != nil {
			return err
		}
		if template.Category == "" {
			template.Category = "General"
		}
	}
	if raw, ok := patch["tags"]; ok {
		template.Tags, err = shared.ParseOptionalTags(raw)
		if err != nil {
			return err
		}
	}
	if raw, ok := patch["is_favorite"]; ok {
		template.IsFavorite, err = shared.ParseOptionalBool(raw)
		if err != nil {
			return err
		}
	}
	if raw, ok := patch["archived"]; ok {
		archived, err := shared.ParseOptionalBool(raw)
		if err != nil {
			return err
		}
		if archived {
			now := shared.Now()
			template.ArchivedAt = &now
		} else {
			template.ArchivedAt = nil
		}
	}
	return nil
}

func sanitizePack(req packInput, partial bool) (ContextPack, error) {
	title := strings.TrimSpace(req.Title)
	if !partial && title == "" {
		return ContextPack{}, errors.New("Context pack title is required")
	}
	pack := ContextPack{
		OrgID:       trimOptional(req.OrgID),
		Title:       title,
		Description: trimOptional(req.Description),
		Tags:        shared.NormalizeTags(req.Tags),
	}
	if req.Archived {
		now := shared.Now()
		pack.ArchivedAt = &now
	}
	return pack, nil
}

func applyPackPatch(pack *ContextPack, patch map[string]json.RawMessage) error {
	var err error
	if raw, ok := patch["org_id"]; ok {
		pack.OrgID, err = shared.ParseOptionalString(raw)
		if err != nil {
			return err
		}
		pack.OrgID = trimOptional(pack.OrgID)
	}
	if raw, ok := patch["title"]; ok {
		pack.Title, err = shared.ParseRequiredString(raw)
		if err != nil || pack.Title == "" {
			return errors.New("Context pack title is required")
		}
	}
	if raw, ok := patch["description"]; ok {
		pack.Description, err = shared.ParseOptionalString(raw)
		if err != nil {
			return err
		}
		pack.Description = trimOptional(pack.Description)
	}
	if raw, ok := patch["tags"]; ok {
		pack.Tags, err = shared.ParseOptionalTags(raw)
		if err != nil {
			return err
		}
	}
	if raw, ok := patch["archived"]; ok {
		archived, err := shared.ParseOptionalBool(raw)
		if err != nil {
			return err
		}
		if archived {
			now := shared.Now()
			pack.ArchivedAt = &now
		} else {
			pack.ArchivedAt = nil
		}
	}
	return nil
}

func sanitizeItems(items []packItemInput) []packItemInput {
	if len(items) == 0 {
		return []packItemInput{}
	}
	out := make([]packItemInput, 0, len(items))
	for i, item := range items {
		item.Title = strings.TrimSpace(item.Title)
		item.Body = strings.TrimSpace(item.Body)
		if item.SortOrder == 0 {
			item.SortOrder = float64(i)
		}
		if item.Title == "" || item.Body == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func insertContextPackItem(ctx context.Context, tx *sql.Tx, packID string, item packItemInput, now string) error {
	enabled := true
	if item.EnabledByDefault != nil {
		enabled = *item.EnabledByDefault
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO context_pack_items
		(id, context_pack_id, title, body, sort_order, enabled_by_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		shared.NewID(),
		packID,
		item.Title,
		item.Body,
		item.SortOrder,
		shared.BoolInt(enabled),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("insert context pack item: %w", err)
	}
	return nil
}

func listContextPackItems(ctx context.Context, q shared.SQLer, packID string) ([]ContextPackItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, context_pack_id, title, body, sort_order, enabled_by_default, created_at, updated_at
		FROM context_pack_items
		WHERE context_pack_id = ?
		ORDER BY sort_order ASC`, packID)
	if err != nil {
		return nil, fmt.Errorf("list context pack items: %w", err)
	}
	defer rows.Close()
	var items []ContextPackItem
	for rows.Next() {
		var item ContextPackItem
		var enabled int
		if err := rows.Scan(&item.ID, &item.ContextPackID, &item.Title, &item.Body, &item.SortOrder, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan context pack item: %w", err)
		}
		item.EnabledByDefault = shared.IntBool(enabled)
		items = append(items, item)
	}
	if items == nil {
		items = []ContextPackItem{}
	}
	return items, rows.Err()
}

func getContextPackOnly(ctx context.Context, q shared.SQLer, id string) (ContextPack, error) {
	var pack ContextPack
	var orgID, description, archivedAt sql.NullString
	var tags string
	err := q.QueryRowContext(ctx, `SELECT id, org_id, title, description, tags, archived_at, created_at, updated_at
		FROM context_packs WHERE id = ?`, id).
		Scan(&pack.ID, &orgID, &pack.Title, &description, &tags, &archivedAt, &pack.CreatedAt, &pack.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ContextPack{}, shared.ErrNotFound
	}
	if err != nil {
		return ContextPack{}, fmt.Errorf("get context pack: %w", err)
	}
	pack.OrgID = shared.FromNullString(orgID)
	pack.Description = shared.FromNullString(description)
	pack.Tags = shared.ParseTags(tags)
	pack.ArchivedAt = shared.FromNullString(archivedAt)
	return pack, nil
}

func scanTemplate(scanner interface {
	Scan(dest ...any) error
}) (Template, error) {
	var template Template
	var orgID, description, archivedAt sql.NullString
	var tags string
	var favorite int
	if err := scanner.Scan(&template.ID, &orgID, &template.Title, &description, &template.Category, &tags, &template.Body, &favorite, &archivedAt, &template.CreatedAt, &template.UpdatedAt); err != nil {
		return Template{}, fmt.Errorf("scan prompt template: %w", err)
	}
	template.OrgID = shared.FromNullString(orgID)
	template.Description = shared.FromNullString(description)
	template.Tags = shared.ParseTags(tags)
	template.IsFavorite = shared.IntBool(favorite)
	template.ArchivedAt = shared.FromNullString(archivedAt)
	return template, nil
}

func scanContextPack(scanner interface {
	Scan(dest ...any) error
}) (ContextPack, error) {
	var pack ContextPack
	var orgID, description, archivedAt sql.NullString
	var tags string
	if err := scanner.Scan(&pack.ID, &orgID, &pack.Title, &description, &tags, &archivedAt, &pack.CreatedAt, &pack.UpdatedAt); err != nil {
		return ContextPack{}, fmt.Errorf("scan context pack: %w", err)
	}
	pack.OrgID = shared.FromNullString(orgID)
	pack.Description = shared.FromNullString(description)
	pack.Tags = shared.ParseTags(tags)
	pack.ArchivedAt = shared.FromNullString(archivedAt)
	return pack, nil
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

func writeBadRequestOrNotFound(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, shared.ErrNotFound) {
		httputil.NotFound(w, r)
		return
	}
	httputil.WriteError(w, http.StatusBadRequest, err.Error())
}
