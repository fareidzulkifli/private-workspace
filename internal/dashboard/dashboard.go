package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/holidays"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Event struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	EventDate string  `json:"event_date"`
	Notes     *string `json:"notes"`
	Color     string  `json:"color"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) ListEvents(ctx context.Context) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, event_date, notes, color, created_at, updated_at
		FROM dashboard_events
		ORDER BY event_date ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list dashboard events: %w", err)
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if events == nil {
		events = []Event{}
	}
	return events, rows.Err()
}

func (r *Repository) CreateEvent(ctx context.Context, input eventInput) (Event, error) {
	event, err := sanitizeEvent(input, false)
	if err != nil {
		return Event{}, err
	}
	now := shared.Now()
	event.ID = shared.NewID()
	event.CreatedAt = now
	event.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `INSERT INTO dashboard_events
		(id, title, event_date, notes, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.Title,
		event.EventDate,
		shared.NullString(event.Notes),
		event.Color,
		event.CreatedAt,
		event.UpdatedAt,
	)
	if err != nil {
		return Event{}, fmt.Errorf("create dashboard event: %w", err)
	}
	return r.GetEvent(ctx, event.ID)
}

func (r *Repository) GetEvent(ctx context.Context, id string) (Event, error) {
	var event Event
	var notes sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, title, event_date, notes, color, created_at, updated_at
		FROM dashboard_events WHERE id = ?`, id).
		Scan(&event.ID, &event.Title, &event.EventDate, &notes, &event.Color, &event.CreatedAt, &event.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, shared.ErrNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("get dashboard event: %w", err)
	}
	event.Notes = shared.FromNullString(notes)
	return event, nil
}

func (r *Repository) UpdateEvent(ctx context.Context, id string, patch map[string]json.RawMessage) (Event, error) {
	current, err := r.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	if err := applyEventPatch(&current, patch); err != nil {
		return Event{}, err
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE dashboard_events
		SET title = ?, event_date = ?, notes = ?, color = ?, updated_at = ?
		WHERE id = ?`,
		current.Title,
		current.EventDate,
		shared.NullString(current.Notes),
		current.Color,
		current.UpdatedAt,
		id,
	)
	if err != nil {
		return Event{}, fmt.Errorf("update dashboard event: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Event{}, shared.ErrNotFound
	}
	return r.GetEvent(ctx, id)
}

func (r *Repository) DeleteEvent(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM dashboard_events WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete dashboard event: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func (r *Repository) DashboardRows(ctx context.Context) ([]dashboardProject, []dashboardTask, []Event, error) {
	projects, err := r.dashboardProjects(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	tasks, err := r.dashboardTasks(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	events, err := r.ListEvents(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return projects, tasks, events, nil
}

func (r *Repository) dashboardProjects(ctx context.Context) ([]dashboardProject, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
			p.id, p.org_id, p.name, p.order_index, p.created_at, p.archived_at,
			o.name, o.slug
		FROM projects p
		JOIN organizations o ON o.id = p.org_id
		WHERE p.archived_at IS NULL
		ORDER BY p.order_index ASC`)
	if err != nil {
		return nil, fmt.Errorf("list dashboard projects: %w", err)
	}
	defer rows.Close()
	var projects []dashboardProject
	for rows.Next() {
		var p dashboardProject
		var archivedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.OrderIndex, &p.CreatedAt, &archivedAt, &p.OrgName, &p.OrgSlug); err != nil {
			return nil, fmt.Errorf("scan dashboard project: %w", err)
		}
		p.ArchivedAt = shared.FromNullString(archivedAt)
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []dashboardProject{}
	}
	return projects, rows.Err()
}

func (r *Repository) dashboardTasks(ctx context.Context) ([]dashboardTask, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
			t.id, t.project_id, t.summary, t.status, t.urgent, t.important,
			t.created_at, t.updated_at, t.completed_at
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE p.archived_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list dashboard tasks: %w", err)
	}
	defer rows.Close()
	var tasks []dashboardTask
	for rows.Next() {
		var t dashboardTask
		var urgent, important int
		var completedAt sql.NullString
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Summary, &t.Status, &urgent, &important, &t.CreatedAt, &t.UpdatedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan dashboard task: %w", err)
		}
		t.Urgent = shared.IntBool(urgent)
		t.Important = shared.IntBool(important)
		t.CompletedAt = shared.FromNullString(completedAt)
		tasks = append(tasks, t)
	}
	if tasks == nil {
		tasks = []dashboardTask{}
	}
	return tasks, rows.Err()
}

type Handler struct {
	repo     *Repository
	holidays holidays.Client
	state    string
}

func NewHandler(database *db.DB, holidayClient holidays.Client, state string) *Handler {
	if state == "" {
		state = holidays.DefaultState
	}
	return &Handler{repo: NewRepository(database), holidays: holidayClient, state: state}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/dashboard", h.GetDashboard)
	r.Get("/api/dashboard/events", h.ListEvents)
	r.Post("/api/dashboard/events", h.CreateEvent)
	r.Patch("/api/dashboard/events/{id}", h.UpdateEvent)
	r.Delete("/api/dashboard/events/{id}", h.DeleteEvent)
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	month := parseCalendarMonth(r.URL.Query().Get("month"))
	today := startOfDay(time.Now())
	upcomingEnd := addDays(today, 30)
	years := uniqueYears([]int{month.Year(), today.Year(), upcomingEnd.Year()})
	var holidayRows []holidays.Holiday
	if h.holidays != nil {
		seen := map[string]bool{}
		for _, year := range years {
			resp := h.holidays.FetchMalaysiaHolidays(r.Context(), h.state, year)
			for _, holiday := range resp.Holidays {
				if !seen[holiday.ID] {
					holidayRows = append(holidayRows, holiday)
					seen[holiday.ID] = true
				}
			}
		}
	}
	projects, tasks, events, err := h.repo.DashboardRows(r.Context())
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, createEmptyDashboard(err.Error(), month, holidayRows))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, buildDashboardData(projects, tasks, events, month, holidayRows))
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.repo.ListEvents(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var input eventInput
	if err := httputil.DecodeJSON(r, 1<<20, &input); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	event, err := h.repo.CreateEvent(r.Context(), input)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, event)
}

func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	event, err := h.repo.UpdateEvent(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			httputil.NotFound(w, r)
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, event)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteEvent(r.Context(), chi.URLParam(r, "id")); err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

type eventInput struct {
	Title     string `json:"title"`
	EventDate string `json:"event_date"`
	Notes     string `json:"notes"`
	Color     string `json:"color"`
}

var allowedColors = map[string]bool{"blue": true, "green": true, "red": true, "violet": true, "slate": true}

func sanitizeEvent(input eventInput, partial bool) (Event, error) {
	event := Event{}
	if !partial || input.Title != "" {
		event.Title = strings.TrimSpace(input.Title)
		if event.Title == "" {
			return Event{}, errors.New("Event title is required")
		}
	}
	if !partial || input.EventDate != "" {
		if !shared.ValidateDateKey(input.EventDate) {
			return Event{}, errors.New("Event date is required")
		}
		event.EventDate = input.EventDate
	}
	if strings.TrimSpace(input.Notes) != "" {
		event.Notes = shared.StringPtr(strings.TrimSpace(input.Notes))
	}
	if !partial || input.Color != "" {
		if allowedColors[input.Color] {
			event.Color = input.Color
		} else {
			event.Color = "blue"
		}
	}
	return event, nil
}

func applyEventPatch(event *Event, patch map[string]json.RawMessage) error {
	if raw, ok := patch["title"]; ok {
		title, err := shared.ParseRequiredString(raw)
		if err != nil || title == "" {
			return errors.New("Event title is required")
		}
		event.Title = title
	}
	if raw, ok := patch["event_date"]; ok {
		date, err := shared.ParseRequiredString(raw)
		if err != nil || !shared.ValidateDateKey(date) {
			return errors.New("Event date is required")
		}
		event.EventDate = date
	}
	if raw, ok := patch["notes"]; ok {
		notes, err := shared.ParseOptionalString(raw)
		if err != nil {
			return err
		}
		if notes != nil {
			trimmed := strings.TrimSpace(*notes)
			if trimmed == "" {
				event.Notes = nil
			} else {
				event.Notes = &trimmed
			}
		} else {
			event.Notes = nil
		}
	}
	if raw, ok := patch["color"]; ok {
		color, err := shared.ParseRequiredString(raw)
		if err != nil {
			return err
		}
		if allowedColors[color] {
			event.Color = color
		} else {
			event.Color = "blue"
		}
	}
	return nil
}

func scanEvent(scanner interface{ Scan(dest ...any) error }) (Event, error) {
	var event Event
	var notes sql.NullString
	if err := scanner.Scan(&event.ID, &event.Title, &event.EventDate, &notes, &event.Color, &event.CreatedAt, &event.UpdatedAt); err != nil {
		return Event{}, fmt.Errorf("scan dashboard event: %w", err)
	}
	event.Notes = shared.FromNullString(notes)
	return event, nil
}

type dashboardProject struct {
	ID         string
	OrgID      string
	Name       string
	OrderIndex float64
	CreatedAt  string
	ArchivedAt *string
	OrgName    string
	OrgSlug    string
}

type dashboardTask struct {
	ID          string
	ProjectID   string
	Summary     string
	Status      string
	Urgent      bool
	Important   bool
	CreatedAt   string
	UpdatedAt   string
	CompletedAt *string
	ProjectName string
	OrgName     string
	OrgSlug     string
}

func buildDashboardData(projects []dashboardProject, tasks []dashboardTask, events []Event, calendarMonth time.Time, holidayRows []holidays.Holiday) map[string]any {
	today := startOfDay(time.Now())
	weekStart := addDays(today, -((int(today.Weekday()) + 6) % 7))
	weekStartKey := dateKey(weekStart)
	todayKey := dateKey(today)

	projectMap := map[string]dashboardProject{}
	for _, project := range projects {
		projectMap[project.ID] = project
	}
	for i := range tasks {
		project := projectMap[tasks[i].ProjectID]
		tasks[i].ProjectName = project.Name
		if project.OrgName == "" {
			tasks[i].OrgName = "Unknown Organization"
		} else {
			tasks[i].OrgName = project.OrgName
		}
		tasks[i].OrgSlug = project.OrgSlug
	}

	activeTasks := filterTasks(tasks, func(task dashboardTask) bool { return task.Status != "Done" })
	highPriorityTasks := filterTasks(activeTasks, func(task dashboardTask) bool { return task.Urgent && task.Important })
	inProgressTasks := filterTasks(tasks, func(task dashboardTask) bool { return task.Status == "In Progress" })
	kivTasks := filterTasks(tasks, func(task dashboardTask) bool { return task.Status == "KIV" })
	completedThisWeek := filterTasks(tasks, func(task dashboardTask) bool {
		key := dateKeyFromStringPtr(task.CompletedAt)
		return key != "" && key >= weekStartKey && key <= todayKey
	})
	activeProjectIDs := map[string]bool{}
	for _, task := range activeTasks {
		activeProjectIDs[task.ProjectID] = true
	}

	sort.Slice(highPriorityTasks, func(i, j int) bool {
		if highPriorityTasks[i].Status != highPriorityTasks[j].Status {
			return highPriorityTasks[i].Status == "In Progress"
		}
		return parseAnyTime(highPriorityTasks[i].CreatedAt).Before(parseAnyTime(highPriorityTasks[j].CreatedAt))
	})
	priorityTasks := []map[string]any{}
	for i, task := range highPriorityTasks {
		if i >= 8 {
			break
		}
		priorityTasks = append(priorityTasks, map[string]any{
			"id":          task.ID,
			"summary":     task.Summary,
			"projectName": task.ProjectName,
			"orgName":     task.OrgName,
			"orgSlug":     task.OrgSlug,
			"status":      task.Status,
			"age":         fmt.Sprintf("%dd open", max(0, diffDays(today, parseAnyTime(task.CreatedAt)))),
		})
	}

	portfolio := buildPortfolio(projects, tasks, today)
	statusCounts := []map[string]any{
		{"label": "In Progress", "value": len(inProgressTasks)},
		{"label": "KIV", "value": len(kivTasks)},
		{"label": "Done", "value": len(filterTasks(tasks, func(task dashboardTask) bool { return task.Status == "Done" }))},
	}
	priorityMix := []map[string]any{
		{"label": "Critical", "value": len(filterTasks(activeTasks, func(task dashboardTask) bool { return task.Urgent && task.Important }))},
		{"label": "Fast Track", "value": len(filterTasks(activeTasks, func(task dashboardTask) bool { return task.Urgent && !task.Important }))},
		{"label": "Strategic", "value": len(filterTasks(activeTasks, func(task dashboardTask) bool { return !task.Urgent && task.Important }))},
		{"label": "Standard", "value": len(filterTasks(activeTasks, func(task dashboardTask) bool { return !task.Urgent && !task.Important }))},
	}

	return map[string]any{
		"error":       nil,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"empty":       len(projects) == 0 && len(events) == 0 && len(holidayRows) == 0,
		"todayKey":    todayKey,
		"kpis": []map[string]any{
			{"label": "Active Tasks", "value": len(activeTasks), "detail": fmt.Sprintf("%d projects in motion", len(activeProjectIDs)), "tone": "neutral"},
			{"label": "High Priority", "value": len(highPriorityTasks), "detail": "Urgent and important", "tone": toneCritical(highPriorityTasks)},
			{"label": "In Progress", "value": len(inProgressTasks), "detail": "Currently moving", "tone": "neutral"},
			{"label": "KIV", "value": len(kivTasks), "detail": "Parked for later", "tone": "neutral"},
			{"label": "Completed Week", "value": len(completedThisWeek), "detail": "Closed since Monday", "tone": "good"},
			{"label": "Active Projects", "value": len(projects), "detail": fmt.Sprintf("%d with open work", len(activeProjectIDs)), "tone": "neutral"},
		},
		"priorityTasks": priorityTasks,
		"holidays":      holidayRows,
		"events":        normalizeEvents(events),
		"calendar":      buildCalendar(tasks, today, calendarMonth, holidayRows),
		"heatmap":       buildHeatmap(tasks, today),
		"portfolio":     portfolio,
		"executionMix": map[string]any{
			"status":   statusCounts,
			"priority": priorityMix,
		},
	}
}

func createEmptyDashboard(errorMessage string, calendarMonth time.Time, holidayRows []holidays.Holiday) map[string]any {
	today := startOfDay(time.Now())
	return map[string]any{
		"error":         nullableError(errorMessage),
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"empty":         true,
		"kpis":          []any{},
		"priorityTasks": []any{},
		"calendar":      buildCalendar(nil, today, calendarMonth, holidayRows),
		"holidays":      holidayRows,
		"events":        []any{},
		"todayKey":      dateKey(today),
		"heatmap":       buildHeatmap(nil, today),
		"portfolio":     []any{},
		"executionMix": map[string]any{
			"status":   []any{},
			"priority": []any{},
		},
	}
}

func buildCalendar(tasks []dashboardTask, today time.Time, calendarMonth time.Time, holidayRows []holidays.Holiday) map[string]any {
	monthStart := time.Date(calendarMonth.Year(), calendarMonth.Month(), 1, 0, 0, 0, 0, time.Local)
	selectedMonthEnd := monthStart.AddDate(0, 1, -1)
	gridStart := addDays(monthStart, -int(monthStart.Weekday()))
	gridEnd := addDays(selectedMonthEnd, 6-int(selectedMonthEnd.Weekday()))
	todayKey := dateKey(today)
	holidaysByDate := map[string][]holidays.Holiday{}
	for _, holiday := range holidayRows {
		holidaysByDate[holiday.Date] = append(holidaysByDate[holiday.Date], holiday)
	}

	var cells []map[string]any
	for date := gridStart; !date.After(gridEnd); date = addDays(date, 1) {
		key := dateKey(date)
		dayHolidays := holidaysByDate[key]
		if dayHolidays == nil {
			dayHolidays = []holidays.Holiday{}
		}
		completed := []map[string]any{}
		movementCount := 0
		for _, task := range tasks {
			if dateKeyFromStringPtr(task.CompletedAt) == key {
				completed = append(completed, map[string]any{
					"id":          task.ID,
					"summary":     task.Summary,
					"projectName": task.ProjectName,
					"orgName":     task.OrgName,
					"orgSlug":     task.OrgSlug,
					"completedAt": task.CompletedAt,
				})
			}
			if taskHasMovementOn(task, key) {
				movementCount++
			}
		}
		activity := 0
		switch {
		case movementCount == 0:
			activity = 0
		case movementCount < 2:
			activity = 1
		case movementCount < 4:
			activity = 2
		default:
			activity = 3
		}
		cells = append(cells, map[string]any{
			"key":            key,
			"day":            date.Day(),
			"weekday":        int(date.Weekday()),
			"inMonth":        date.Month() == monthStart.Month(),
			"isWeekend":      date.Weekday() == time.Sunday || date.Weekday() == time.Saturday,
			"isToday":        key == todayKey,
			"movementCount":  movementCount,
			"completedCount": len(completed),
			"holidayCount":   len(dayHolidays),
			"holidays":       dayHolidays,
			"completedTasks": completed,
			"activityLevel":  activity,
		})
	}
	var weeks [][]map[string]any
	for i := 0; i < len(cells); i += 7 {
		weeks = append(weeks, cells[i:i+7])
	}
	state := holidays.DefaultState
	if len(holidayRows) > 0 {
		state = holidayRows[0].State
	}
	return map[string]any{
		"label":            monthStart.Format("January 2006"),
		"monthKey":         monthKey(monthStart),
		"previousMonthKey": monthKey(monthStart.AddDate(0, -1, 0)),
		"nextMonthKey":     monthKey(monthStart.AddDate(0, 1, 0)),
		"todayMonthKey":    monthKey(today),
		"holidayState":     state,
		"weekdays":         []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
		"weeks":            weeks,
	}
}

func buildHeatmap(tasks []dashboardTask, today time.Time) map[string]any {
	todayKey := dateKey(today)
	first := addDays(addDays(today, -364), -int(addDays(today, -364).Weekday()))
	counts := map[string]int{}
	for _, task := range tasks {
		for _, key := range taskMovementKeys(task) {
			if key <= todayKey {
				counts[key]++
			}
		}
	}
	cells := []map[string]any{}
	for date := first; !date.After(today); date = addDays(date, 1) {
		key := dateKey(date)
		count := counts[key]
		level := 0
		switch {
		case count == 0:
			level = 0
		case count < 2:
			level = 1
		case count < 4:
			level = 2
		case count < 7:
			level = 3
		default:
			level = 4
		}
		cells = append(cells, map[string]any{"key": key, "count": count, "level": level})
	}
	var weeks [][]map[string]any
	for i := 0; i < len(cells); i += 7 {
		end := min(i+7, len(cells))
		weeks = append(weeks, cells[i:end])
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	return map[string]any{"weeks": weeks, "totalMovements": total}
}

func buildPortfolio(projects []dashboardProject, tasks []dashboardTask, today time.Time) []map[string]any {
	portfolio := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		projectTasks := filterTasks(tasks, func(task dashboardTask) bool { return task.ProjectID == project.ID })
		pending := filterTasks(projectTasks, func(task dashboardTask) bool { return task.Status != "Done" })
		high := filterTasks(pending, func(task dashboardTask) bool { return task.Urgent && task.Important })
		kiv := filterTasks(pending, func(task dashboardTask) bool { return task.Status == "KIV" })
		lastMovement := parseAnyTime(project.CreatedAt)
		for _, task := range projectTasks {
			for _, value := range []string{task.CreatedAt, task.UpdatedAt, stringFromPtr(task.CompletedAt)} {
				t := parseAnyTime(value)
				if t.After(lastMovement) {
					lastMovement = t
				}
			}
		}
		stagnant := len(pending) > 0 && diffDays(today, lastMovement) >= 14
		health := "Clear"
		if len(high) > 0 {
			health = "Critical"
		} else if stagnant {
			health = "At Risk"
		} else if len(pending) > 0 {
			health = "Active"
		}
		orgName := project.OrgName
		if orgName == "" {
			orgName = "Unknown Organization"
		}
		portfolio = append(portfolio, map[string]any{
			"id":                project.ID,
			"name":              project.Name,
			"orgName":           orgName,
			"orgSlug":           project.OrgSlug,
			"activeTasks":       len(pending),
			"highPriorityTasks": len(high),
			"kivTasks":          len(kiv),
			"lastMovement":      formatAgo(lastMovement, today),
			"health":            health,
		})
	}
	rank := map[string]int{"Critical": 0, "At Risk": 1, "Active": 2, "Clear": 3}
	sort.Slice(portfolio, func(i, j int) bool {
		a := portfolio[i]
		b := portfolio[j]
		if rank[a["health"].(string)] != rank[b["health"].(string)] {
			return rank[a["health"].(string)] < rank[b["health"].(string)]
		}
		if a["highPriorityTasks"].(int) != b["highPriorityTasks"].(int) {
			return a["highPriorityTasks"].(int) > b["highPriorityTasks"].(int)
		}
		return a["activeTasks"].(int) > b["activeTasks"].(int)
	})
	return portfolio
}

func filterTasks(tasks []dashboardTask, keep func(dashboardTask) bool) []dashboardTask {
	out := []dashboardTask{}
	for _, task := range tasks {
		if keep(task) {
			out = append(out, task)
		}
	}
	return out
}

func normalizeEvents(events []Event) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, map[string]any{
			"id":         event.ID,
			"title":      event.Title,
			"event_date": event.EventDate,
			"notes":      stringFromPtr(event.Notes),
			"color":      event.Color,
			"created_at": event.CreatedAt,
			"updated_at": event.UpdatedAt,
		})
	}
	return out
}

func taskHasMovementOn(task dashboardTask, key string) bool {
	for _, movementKey := range taskMovementKeys(task) {
		if movementKey == key {
			return true
		}
	}
	return false
}

func taskMovementKeys(task dashboardTask) []string {
	seen := map[string]bool{}
	for _, key := range []string{
		dateKeyFromString(task.CreatedAt),
		dateKeyFromString(task.UpdatedAt),
		dateKeyFromStringPtr(task.CompletedAt),
	} {
		if key != "" {
			seen[key] = true
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	return out
}

func parseCalendarMonth(value string) time.Time {
	if !shared.ValidateMonthKey(value) {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	}
	return time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.Local)
}

func dateKeyFromString(value string) string {
	if value == "" {
		return ""
	}
	if len(value) == 10 && shared.ValidateDateKey(value) {
		return value
	}
	t := parseAnyTime(value)
	if t.IsZero() {
		return ""
	}
	return dateKey(t)
}

func dateKeyFromStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return dateKeyFromString(*value)
}

func parseAnyTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if len(value) == 10 {
		t, _ := time.ParseInLocation("2006-01-02", value, time.Local)
		return t
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func startOfDay(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
}

func addDays(t time.Time, days int) time.Time {
	return t.AddDate(0, 0, days)
}

func dateKey(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.Local()
	return fmt.Sprintf("%04d-%02d-%02d", local.Year(), local.Month(), local.Day())
}

func monthKey(t time.Time) string {
	local := t.Local()
	return fmt.Sprintf("%04d-%02d", local.Year(), local.Month())
}

func diffDays(a time.Time, b time.Time) int {
	if a.IsZero() || b.IsZero() {
		return 0
	}
	a = startOfDay(a)
	b = startOfDay(b)
	return int(a.Sub(b).Hours() / 24)
}

func formatAgo(value time.Time, today time.Time) string {
	if value.IsZero() {
		return "No movement"
	}
	days := diffDays(today, value)
	if days <= 0 {
		return "Today"
	}
	if days == 1 {
		return "Yesterday"
	}
	if days < 30 {
		return fmt.Sprintf("%dd ago", days)
	}
	return fmt.Sprintf("%dmo ago", days/30)
}

func uniqueYears(years []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, year := range years {
		if !seen[year] {
			out = append(out, year)
			seen[year] = true
		}
	}
	return out
}

func nullableError(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func stringFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toneCritical(tasks []dashboardTask) string {
	if len(tasks) > 0 {
		return "critical"
	}
	return "neutral"
}
