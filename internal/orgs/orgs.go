package orgs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"private-workspace/internal/db"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Organization struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Slug       string  `json:"slug"`
	OrderIndex float64 `json:"order_index"`
	CreatedAt  string  `json:"created_at"`
}

type NavOrganization struct {
	Organization
	Projects []NavProject `json:"projects"`
}

type NavProject struct {
	ID                   string  `json:"id"`
	OrgID                string  `json:"org_id"`
	Name                 string  `json:"name"`
	OrderIndex           float64 `json:"order_index"`
	CreatedAt            string  `json:"created_at"`
	DescriptionMarkdown  *string `json:"description_markdown"`
	Goal                 *string `json:"goal"`
	ContextMarkdown      *string `json:"context_markdown"`
	ProjectType          string  `json:"project_type"`
	AIInstructions       *string `json:"ai_instructions"`
	CurrentFocus         *string `json:"current_focus"`
	TargetDate           *string `json:"target_date"`
	ArchivedAt           *string `json:"archived_at"`
	IncompleteTasksCount int     `json:"incomplete_tasks_count"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) List(ctx context.Context) ([]Organization, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, slug, order_index, created_at
		FROM organizations
		ORDER BY order_index ASC`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		org, err := scanOrganization(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if orgs == nil {
		orgs = []Organization{}
	}
	return orgs, rows.Err()
}

func (r *Repository) Create(ctx context.Context, name string, orderIndex float64) (Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Organization{}, errors.New("Name is required")
	}
	now := shared.Now()
	org := Organization{
		ID:         shared.NewID(),
		Name:       name,
		Slug:       ToSlug(name),
		OrderIndex: orderIndex,
		CreatedAt:  now,
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO organizations (id, name, slug, order_index, created_at)
		VALUES (?, ?, ?, ?, ?)`, org.ID, org.Name, org.Slug, org.OrderIndex, org.CreatedAt)
	if err != nil {
		if isUniqueConstraint(err) {
			return Organization{}, fmt.Errorf("organization slug already exists")
		}
		return Organization{}, fmt.Errorf("create organization: %w", err)
	}
	return org, nil
}

func (r *Repository) Get(ctx context.Context, id string) (Organization, error) {
	return r.getWhere(ctx, "id = ?", id)
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (Organization, error) {
	return r.getWhere(ctx, "slug = ?", slug)
}

func (r *Repository) getWhere(ctx context.Context, where string, arg any) (Organization, error) {
	var org Organization
	err := r.db.QueryRowContext(ctx, `SELECT id, name, slug, order_index, created_at
		FROM organizations WHERE `+where, arg).
		Scan(&org.ID, &org.Name, &org.Slug, &org.OrderIndex, &org.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, shared.ErrNotFound
	}
	if err != nil {
		return Organization{}, fmt.Errorf("get organization: %w", err)
	}
	return org, nil
}

func (r *Repository) Update(ctx context.Context, id string, patch map[string]json.RawMessage) (Organization, error) {
	current, err := r.Get(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if raw, ok := patch["name"]; ok {
		name, err := shared.ParseRequiredString(raw)
		if err != nil || name == "" {
			return Organization{}, errors.New("Name is required")
		}
		current.Name = name
		current.Slug = ToSlug(name)
	}
	if raw, ok := patch["order_index"]; ok {
		current.OrderIndex, err = shared.ParseOptionalFloat(raw)
		if err != nil {
			return Organization{}, errors.New("order_index must be a number")
		}
	}
	result, err := r.db.ExecContext(ctx, `UPDATE organizations
		SET name = ?, slug = ?, order_index = ?
		WHERE id = ?`, current.Name, current.Slug, current.OrderIndex, current.ID)
	if err != nil {
		if isUniqueConstraint(err) {
			return Organization{}, fmt.Errorf("organization slug already exists")
		}
		return Organization{}, fmt.Errorf("update organization: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Organization{}, shared.ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM organizations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete organization: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func (r *Repository) Nav(ctx context.Context) ([]NavOrganization, error) {
	orgs, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT
			p.id, p.org_id, p.name, p.order_index, p.created_at,
			p.description_markdown, p.goal, p.context_markdown, p.project_type,
			p.ai_instructions, p.current_focus, p.target_date, p.archived_at,
			COALESCE(SUM(CASE WHEN t.status != 'Done' THEN 1 ELSE 0 END), 0) AS incomplete_tasks_count
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id
		WHERE p.archived_at IS NULL
		GROUP BY p.id
		ORDER BY p.order_index ASC`)
	if err != nil {
		return nil, fmt.Errorf("list nav projects: %w", err)
	}
	defer rows.Close()

	byOrg := make(map[string][]NavProject)
	for rows.Next() {
		var project NavProject
		var description, goal, contextMarkdown, aiInstructions, currentFocus, targetDate, archivedAt sql.NullString
		if err := rows.Scan(
			&project.ID,
			&project.OrgID,
			&project.Name,
			&project.OrderIndex,
			&project.CreatedAt,
			&description,
			&goal,
			&contextMarkdown,
			&project.ProjectType,
			&aiInstructions,
			&currentFocus,
			&targetDate,
			&archivedAt,
			&project.IncompleteTasksCount,
		); err != nil {
			return nil, fmt.Errorf("scan nav project: %w", err)
		}
		project.DescriptionMarkdown = shared.FromNullString(description)
		project.Goal = shared.FromNullString(goal)
		project.ContextMarkdown = shared.FromNullString(contextMarkdown)
		project.AIInstructions = shared.FromNullString(aiInstructions)
		project.CurrentFocus = shared.FromNullString(currentFocus)
		project.TargetDate = shared.FromNullString(targetDate)
		project.ArchivedAt = shared.FromNullString(archivedAt)
		byOrg[project.OrgID] = append(byOrg[project.OrgID], project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]NavOrganization, 0, len(orgs))
	for _, org := range orgs {
		projects := byOrg[org.ID]
		if projects == nil {
			projects = []NavProject{}
		}
		out = append(out, NavOrganization{
			Organization: org,
			Projects:     projects,
		})
	}
	return out, nil
}

type Handler struct {
	repo *Repository
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{repo: NewRepository(database)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/nav", h.Nav)
	r.Get("/api/orgs", h.List)
	r.Post("/api/orgs", h.Create)
	r.Get("/api/orgs/by-slug/{slug}", h.GetBySlug)
	r.Get("/api/orgs/{id}", h.Get)
	r.Patch("/api/orgs/{id}", h.Update)
	r.Delete("/api/orgs/{id}", h.Delete)
}

func (h *Handler) Nav(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.Nav(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, items)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.repo.List(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, orgs)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		OrderIndex *float64 `json:"order_index"`
	}
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	orderIndex := 0.0
	if req.OrderIndex != nil {
		orderIndex = *req.OrderIndex
	}
	org, err := h.repo.Create(r.Context(), req.Name, orderIndex)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, org)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	org, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, org)
}

func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	org, err := h.repo.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, org)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	org, err := h.repo.Update(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			httputil.NotFound(w, r)
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, org)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

var slugReplaceRE = regexp.MustCompile(`[^a-z0-9\s-]`)
var slugSpaceRE = regexp.MustCompile(`\s+`)
var slugDashRE = regexp.MustCompile(`-+`)

func ToSlug(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = slugReplaceRE.ReplaceAllString(value, "")
	value = slugSpaceRE.ReplaceAllString(value, "-")
	value = slugDashRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func scanOrganization(scanner interface {
	Scan(dest ...any) error
}) (Organization, error) {
	var org Organization
	if err := scanner.Scan(&org.ID, &org.Name, &org.Slug, &org.OrderIndex, &org.CreatedAt); err != nil {
		return Organization{}, fmt.Errorf("scan organization: %w", err)
	}
	return org, nil
}

func isUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
}
