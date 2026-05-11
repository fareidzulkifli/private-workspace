package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Task struct {
	ID            string       `json:"id"`
	ProjectID     string       `json:"project_id"`
	Summary       string       `json:"summary"`
	NotesMarkdown *string      `json:"notes_markdown"`
	Status        string       `json:"status"`
	Urgent        bool         `json:"urgent"`
	Important     bool         `json:"important"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	DueDate       *string      `json:"due_date"`
	OrderIndex    float64      `json:"order_index"`
	CompletedAt   *string      `json:"completed_at"`
	Projects      *TaskProject `json:"projects,omitempty"`
}

type TaskProject struct {
	OrgID      string  `json:"org_id"`
	ArchivedAt *string `json:"archived_at"`
}

type Attachment struct {
	ID         string `json:"id"`
	TaskID     string `json:"task_id"`
	Filename   string `json:"filename"`
	R2Key      string `json:"r2_key"`
	MimeType   string `json:"mime_type"`
	SizeBytes  int64  `json:"size_bytes"`
	UploadedAt string `json:"uploaded_at"`
	URL        string `json:"url,omitempty"`
}

type ObjectStore interface {
	PresignGetObject(ctx context.Context, key string, expires time.Duration) (string, error)
	DeleteObject(ctx context.Context, key string) error
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) List(ctx context.Context, projectID string, orgID string) ([]Task, error) {
	args := []any{}
	where := ""
	if strings.TrimSpace(projectID) != "" {
		where = "WHERE t.project_id = ?"
		args = append(args, projectID)
	} else if strings.TrimSpace(orgID) != "" {
		where = "WHERE p.org_id = ? AND p.archived_at IS NULL"
		args = append(args, orgID)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT
			t.id, t.project_id, t.summary, t.notes_markdown, t.status, t.urgent, t.important,
			t.created_at, t.updated_at, t.due_date, t.order_index, t.completed_at,
			p.org_id, p.archived_at
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		`+where+`
		ORDER BY t.order_index ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows, true)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if tasks == nil {
		tasks = []Task{}
	}
	return tasks, rows.Err()
}

func (r *Repository) Create(ctx context.Context, req CreateRequest) (Task, error) {
	req.Summary = strings.TrimSpace(req.Summary)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	if req.Summary == "" || req.ProjectID == "" {
		return Task{}, errors.New("Summary and Project ID are required")
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "In Progress"
	}
	now := shared.Now()
	var completedAt *string
	if status == "Done" {
		completedAt = &now
	}
	id := shared.NewID()
	_, err := r.db.ExecContext(ctx, `INSERT INTO tasks
		(id, project_id, summary, notes_markdown, status, urgent, important,
		 created_at, updated_at, due_date, order_index, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		req.ProjectID,
		req.Summary,
		sql.NullString{String: strings.TrimSpace(req.NotesMarkdown), Valid: strings.TrimSpace(req.NotesMarkdown) != ""},
		status,
		shared.BoolInt(req.Urgent),
		shared.BoolInt(req.Important),
		now,
		now,
		shared.NullString(req.DueDate),
		req.OrderIndex,
		shared.NullString(completedAt),
	)
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *Repository) Get(ctx context.Context, id string) (Task, error) {
	return getTask(ctx, r.db, id)
}

func (r *Repository) Update(ctx context.Context, id string, patch map[string]json.RawMessage) (Task, error) {
	current, err := r.Get(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if raw, ok := patch["summary"]; ok {
		summary, err := shared.ParseRequiredString(raw)
		if err != nil || summary == "" {
			return Task{}, errors.New("Summary is required")
		}
		current.Summary = summary
	}
	if raw, ok := patch["order_index"]; ok {
		current.OrderIndex, err = shared.ParseOptionalFloat(raw)
		if err != nil {
			return Task{}, errors.New("order_index must be a number")
		}
	}
	if raw, ok := patch["notes_markdown"]; ok {
		current.NotesMarkdown, err = shared.ParseOptionalString(raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := patch["status"]; ok {
		status, err := shared.ParseRequiredString(raw)
		if err != nil || status == "" {
			return Task{}, errors.New("status is required")
		}
		current.Status = status
		if status == "Done" {
			now := shared.Now()
			current.CompletedAt = &now
		} else {
			current.CompletedAt = nil
		}
	}
	if raw, ok := patch["urgent"]; ok {
		current.Urgent, err = shared.ParseOptionalBool(raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := patch["important"]; ok {
		current.Important, err = shared.ParseOptionalBool(raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := patch["due_date"]; ok {
		current.DueDate, err = shared.ParseOptionalString(raw)
		if err != nil {
			return Task{}, err
		}
	}
	if raw, ok := patch["completed_at"]; ok {
		completedAt, err := parseCompletedAt(raw)
		if err != nil {
			return Task{}, err
		}
		if completedAt == nil {
			if current.Status == "Done" {
				return Task{}, errors.New("completed_at is required for completed tasks")
			}
		} else if current.Status != "Done" {
			return Task{}, errors.New("completed_at can only be set for completed tasks")
		}
		current.CompletedAt = completedAt
	}
	if raw, ok := patch["project_id"]; ok {
		projectID, err := shared.ParseRequiredString(raw)
		if err != nil || projectID == "" {
			return Task{}, errors.New("project_id is required")
		}
		current.ProjectID = projectID
	}
	current.UpdatedAt = shared.Now()

	result, err := r.db.ExecContext(ctx, `UPDATE tasks
		SET project_id = ?, summary = ?, notes_markdown = ?, status = ?, urgent = ?,
			important = ?, updated_at = ?, due_date = ?, order_index = ?, completed_at = ?
		WHERE id = ?`,
		current.ProjectID,
		current.Summary,
		shared.NullString(current.NotesMarkdown),
		current.Status,
		shared.BoolInt(current.Urgent),
		shared.BoolInt(current.Important),
		current.UpdatedAt,
		shared.NullString(current.DueDate),
		current.OrderIndex,
		shared.NullString(current.CompletedAt),
		id,
	)
	if err != nil {
		return Task{}, fmt.Errorf("update task: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Task{}, shared.ErrNotFound
	}
	return r.Get(ctx, id)
}

func parseCompletedAt(raw json.RawMessage) (*string, error) {
	completedAt, err := shared.ParseOptionalString(raw)
	if err != nil {
		return nil, errors.New("completed_at must be a timestamp")
	}
	if completedAt == nil || strings.TrimSpace(*completedAt) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*completedAt))
	if err != nil {
		return nil, errors.New("completed_at must be an RFC3339 timestamp")
	}
	normalized := shared.FormatTime(parsed)
	return &normalized, nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func (r *Repository) ListAttachments(ctx context.Context, taskID string) ([]Attachment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, task_id, filename, r2_key, mime_type, size_bytes, uploaded_at
		FROM task_attachments
		WHERE task_id = ?
		ORDER BY uploaded_at ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	var attachments []Attachment
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	if attachments == nil {
		attachments = []Attachment{}
	}
	return attachments, rows.Err()
}

func (r *Repository) CreateAttachment(ctx context.Context, taskID string, req AttachmentRequest) (Attachment, error) {
	req.Filename = strings.TrimSpace(req.Filename)
	req.R2Key = strings.TrimSpace(req.R2Key)
	req.MimeType = strings.TrimSpace(req.MimeType)
	if req.Filename == "" || req.R2Key == "" || req.MimeType == "" || req.SizeBytes <= 0 {
		return Attachment{}, errors.New("Missing attachment data")
	}
	now := shared.Now()
	id := shared.NewID()
	_, err := r.db.ExecContext(ctx, `INSERT INTO task_attachments
		(id, task_id, filename, r2_key, mime_type, size_bytes, uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, req.Filename, req.R2Key, req.MimeType, req.SizeBytes, now)
	if err != nil {
		return Attachment{}, fmt.Errorf("create attachment: %w", err)
	}
	return r.GetAttachment(ctx, id)
}

func (r *Repository) GetAttachment(ctx context.Context, attachmentID string) (Attachment, error) {
	var attachment Attachment
	err := r.db.QueryRowContext(ctx, `SELECT id, task_id, filename, r2_key, mime_type, size_bytes, uploaded_at
		FROM task_attachments WHERE id = ?`, attachmentID).
		Scan(&attachment.ID, &attachment.TaskID, &attachment.Filename, &attachment.R2Key, &attachment.MimeType, &attachment.SizeBytes, &attachment.UploadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Attachment{}, shared.ErrNotFound
	}
	if err != nil {
		return Attachment{}, fmt.Errorf("get attachment: %w", err)
	}
	return attachment, nil
}

func (r *Repository) DeleteAttachmentMetadata(ctx context.Context, attachmentID string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM task_attachments WHERE id = ?", attachmentID)
	if err != nil {
		return fmt.Errorf("delete attachment metadata: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

type CreateRequest struct {
	Summary       string  `json:"summary"`
	ProjectID     string  `json:"project_id"`
	OrderIndex    float64 `json:"order_index"`
	NotesMarkdown string  `json:"notes_markdown"`
	Status        string  `json:"status"`
	Urgent        bool    `json:"urgent"`
	Important     bool    `json:"important"`
	DueDate       *string `json:"due_date"`
}

type AttachmentRequest struct {
	Filename  string `json:"filename"`
	R2Key     string `json:"r2_key"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

type Handler struct {
	repo  *Repository
	store ObjectStore
}

func NewHandler(database *db.DB, store ObjectStore) *Handler {
	return &Handler{repo: NewRepository(database), store: store}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/tasks", h.List)
	r.Post("/api/tasks", h.Create)
	r.Patch("/api/tasks/{id}", h.Update)
	r.Delete("/api/tasks/{id}", h.Delete)
	r.Get("/api/tasks/{id}/attachments", h.ListAttachments)
	r.Post("/api/tasks/{id}/attachments", h.CreateAttachment)
	r.Delete("/api/tasks/{id}/attachments", h.DeleteAttachment)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.repo.List(r.Context(), r.URL.Query().Get("project_id"), r.URL.Query().Get("org_id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, tasks)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	task, err := h.repo.Create(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, task)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	task, err := h.repo.Update(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			httputil.NotFound(w, r)
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, task)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	attachments, err := h.repo.ListAttachments(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	if h.store != nil {
		for i := range attachments {
			url, err := h.store.PresignGetObject(r.Context(), attachments[i].R2Key, time.Hour)
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			attachments[i].URL = url
		}
	}
	httputil.WriteJSON(w, http.StatusOK, attachments)
}

func (h *Handler) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	var req AttachmentRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	attachment, err := h.repo.CreateAttachment(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, attachment)
}

func (h *Handler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentID := strings.TrimSpace(r.URL.Query().Get("attachment_id"))
	if attachmentID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Attachment ID is required")
		return
	}
	attachment, err := h.repo.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	if h.store != nil {
		if err := h.store.DeleteObject(r.Context(), attachment.R2Key); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := h.repo.DeleteAttachmentMetadata(r.Context(), attachmentID); err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func getTask(ctx context.Context, q shared.SQLer, id string) (Task, error) {
	row := q.QueryRowContext(ctx, `SELECT
			t.id, t.project_id, t.summary, t.notes_markdown, t.status, t.urgent, t.important,
			t.created_at, t.updated_at, t.due_date, t.order_index, t.completed_at,
			p.org_id, p.archived_at
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = ?`, id)
	task, err := scanTask(row, true)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, shared.ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	return task, nil
}

func scanTask(scanner interface {
	Scan(dest ...any) error
}, includeProject bool) (Task, error) {
	var task Task
	var notes, due, completed, archived sql.NullString
	var urgent, important int
	dest := []any{
		&task.ID,
		&task.ProjectID,
		&task.Summary,
		&notes,
		&task.Status,
		&urgent,
		&important,
		&task.CreatedAt,
		&task.UpdatedAt,
		&due,
		&task.OrderIndex,
		&completed,
	}
	var project TaskProject
	if includeProject {
		dest = append(dest, &project.OrgID, &archived)
	}
	if err := scanner.Scan(dest...); err != nil {
		return Task{}, fmt.Errorf("scan task: %w", err)
	}
	task.NotesMarkdown = shared.FromNullString(notes)
	task.Urgent = shared.IntBool(urgent)
	task.Important = shared.IntBool(important)
	task.DueDate = shared.FromNullString(due)
	task.CompletedAt = shared.FromNullString(completed)
	if includeProject {
		project.ArchivedAt = shared.FromNullString(archived)
		task.Projects = &project
	}
	return task, nil
}

func scanAttachment(scanner interface {
	Scan(dest ...any) error
}) (Attachment, error) {
	var attachment Attachment
	if err := scanner.Scan(&attachment.ID, &attachment.TaskID, &attachment.Filename, &attachment.R2Key, &attachment.MimeType, &attachment.SizeBytes, &attachment.UploadedAt); err != nil {
		return Attachment{}, fmt.Errorf("scan attachment: %w", err)
	}
	return attachment, nil
}
