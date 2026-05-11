package projects

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

type Project struct {
	ID                  string  `json:"id"`
	OrgID               string  `json:"org_id"`
	Name                string  `json:"name"`
	OrderIndex          float64 `json:"order_index"`
	CreatedAt           string  `json:"created_at"`
	DescriptionMarkdown *string `json:"description_markdown"`
	Goal                *string `json:"goal"`
	ContextMarkdown     *string `json:"context_markdown"`
	ProjectType         string  `json:"project_type"`
	AIInstructions      *string `json:"ai_instructions"`
	CurrentFocus        *string `json:"current_focus"`
	TargetDate          *string `json:"target_date"`
	ArchivedAt          *string `json:"archived_at"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) List(ctx context.Context, orgID string, archived bool) ([]Project, error) {
	args := []any{}
	where := "WHERE archived_at IS NULL"
	order := "ORDER BY order_index ASC"
	if archived {
		where = "WHERE archived_at IS NOT NULL"
		order = "ORDER BY archived_at DESC"
	}
	if strings.TrimSpace(orgID) != "" {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, org_id, name, order_index, created_at,
			description_markdown, goal, context_markdown, project_type, ai_instructions,
			current_focus, target_date, archived_at
		FROM projects `+where+" "+order, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if projects == nil {
		projects = []Project{}
	}
	return projects, rows.Err()
}

func (r *Repository) Create(ctx context.Context, req CreateRequest) (Project, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.OrgID = strings.TrimSpace(req.OrgID)
	if req.Name == "" || req.OrgID == "" {
		return Project{}, errors.New("Name and Organization ID are required")
	}
	now := shared.Now()
	project := Project{
		ID:          shared.NewID(),
		OrgID:       req.OrgID,
		Name:        req.Name,
		OrderIndex:  req.OrderIndex,
		CreatedAt:   now,
		ProjectType: "Work",
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO projects
		(id, org_id, name, order_index, created_at, project_type)
		VALUES (?, ?, ?, ?, ?, ?)`,
		project.ID, project.OrgID, project.Name, project.OrderIndex, project.CreatedAt, project.ProjectType)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return r.Get(ctx, project.ID)
}

func (r *Repository) Get(ctx context.Context, id string) (Project, error) {
	return getProject(ctx, r.db, id)
}

func (r *Repository) Update(ctx context.Context, id string, patch map[string]json.RawMessage) (Project, error) {
	var updated Project
	err := r.db.Tx(ctx, func(tx *sql.Tx) error {
		current, err := getProject(ctx, tx, id)
		if err != nil {
			return err
		}
		if raw, ok := patch["name"]; ok {
			name, err := shared.ParseRequiredString(raw)
			if err != nil || name == "" {
				return errors.New("Name is required")
			}
			current.Name = name
		}
		if raw, ok := patch["order_index"]; ok {
			current.OrderIndex, err = shared.ParseOptionalFloat(raw)
			if err != nil {
				return errors.New("order_index must be a number")
			}
		}
		if raw, ok := patch["description_markdown"]; ok {
			current.DescriptionMarkdown, err = shared.ParseOptionalString(raw)
			if err != nil {
				return err
			}
		}
		if raw, ok := patch["goal"]; ok {
			current.Goal, err = shared.ParseOptionalString(raw)
			if err != nil {
				return err
			}
		}
		if raw, ok := patch["context_markdown"]; ok {
			current.ContextMarkdown, err = shared.ParseOptionalString(raw)
			if err != nil {
				return err
			}
		}
		if raw, ok := patch["project_type"]; ok {
			projectType, err := shared.ParseRequiredString(raw)
			if err != nil {
				return err
			}
			if projectType == "" {
				projectType = "Work"
			}
			current.ProjectType = projectType
		}
		if raw, ok := patch["ai_instructions"]; ok {
			current.AIInstructions, err = shared.ParseOptionalString(raw)
			if err != nil {
				return err
			}
		}
		if raw, ok := patch["current_focus"]; ok {
			current.CurrentFocus, err = shared.ParseOptionalString(raw)
			if err != nil {
				return err
			}
		}
		if raw, ok := patch["target_date"]; ok {
			current.TargetDate, err = shared.ParseOptionalString(raw)
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
				var count int
				if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE project_id = ? AND status != 'Done'", id).Scan(&count); err != nil {
					return fmt.Errorf("check project tasks: %w", err)
				}
				if count > 0 {
					return errors.New("Projects can only be archived when all tasks are done.")
				}
				now := shared.Now()
				current.ArchivedAt = &now
			} else {
				current.ArchivedAt = nil
			}
		}

		result, err := tx.ExecContext(ctx, `UPDATE projects
			SET name = ?, order_index = ?, description_markdown = ?, goal = ?,
				context_markdown = ?, project_type = ?, ai_instructions = ?,
				current_focus = ?, target_date = ?, archived_at = ?
			WHERE id = ?`,
			current.Name,
			current.OrderIndex,
			shared.NullString(current.DescriptionMarkdown),
			shared.NullString(current.Goal),
			shared.NullString(current.ContextMarkdown),
			current.ProjectType,
			shared.NullString(current.AIInstructions),
			shared.NullString(current.CurrentFocus),
			shared.NullString(current.TargetDate),
			shared.NullString(current.ArchivedAt),
			id,
		)
		if err != nil {
			return fmt.Errorf("update project: %w", err)
		}
		if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
			return shared.ErrNotFound
		}
		updated, err = getProject(ctx, tx, id)
		return err
	})
	if err != nil {
		return Project{}, err
	}
	return updated, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

type CreateRequest struct {
	Name       string  `json:"name"`
	OrgID      string  `json:"org_id"`
	OrderIndex float64 `json:"order_index"`
}

type Handler struct {
	repo *Repository
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{repo: NewRepository(database)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/projects", h.List)
	r.Post("/api/projects", h.Create)
	r.Get("/api/projects/{id}", h.Get)
	r.Patch("/api/projects/{id}", h.Update)
	r.Delete("/api/projects/{id}", h.Delete)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projects, err := h.repo.List(r.Context(), r.URL.Query().Get("org_id"), shared.ParseBoolQuery(r, "archived"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, projects)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	project, err := h.repo.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, project)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	project, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, project)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	project, err := h.repo.Update(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			httputil.NotFound(w, r)
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, project)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func getProject(ctx context.Context, q shared.SQLer, id string) (Project, error) {
	var project Project
	var description, goal, contextMarkdown, aiInstructions, currentFocus, targetDate, archivedAt sql.NullString
	err := q.QueryRowContext(ctx, `SELECT id, org_id, name, order_index, created_at,
			description_markdown, goal, context_markdown, project_type, ai_instructions,
			current_focus, target_date, archived_at
		FROM projects WHERE id = ?`, id).
		Scan(
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
		)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, shared.ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	project.DescriptionMarkdown = shared.FromNullString(description)
	project.Goal = shared.FromNullString(goal)
	project.ContextMarkdown = shared.FromNullString(contextMarkdown)
	project.AIInstructions = shared.FromNullString(aiInstructions)
	project.CurrentFocus = shared.FromNullString(currentFocus)
	project.TargetDate = shared.FromNullString(targetDate)
	project.ArchivedAt = shared.FromNullString(archivedAt)
	return project, nil
}

func scanProject(scanner interface {
	Scan(dest ...any) error
}) (Project, error) {
	var project Project
	var description, goal, contextMarkdown, aiInstructions, currentFocus, targetDate, archivedAt sql.NullString
	if err := scanner.Scan(
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
	); err != nil {
		return Project{}, fmt.Errorf("scan project: %w", err)
	}
	project.DescriptionMarkdown = shared.FromNullString(description)
	project.Goal = shared.FromNullString(goal)
	project.ContextMarkdown = shared.FromNullString(contextMarkdown)
	project.AIInstructions = shared.FromNullString(aiInstructions)
	project.CurrentFocus = shared.FromNullString(currentFocus)
	project.TargetDate = shared.FromNullString(targetDate)
	project.ArchivedAt = shared.FromNullString(archivedAt)
	return project, nil
}
