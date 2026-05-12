package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"private-workspace/internal/shared"
)

func insertMonth(ctx context.Context, q shared.SQLer, month Month) error {
	_, err := q.ExecContext(ctx, `INSERT INTO wallet_months
		(id, month, opening_balance_cents, wallet_balance_cents, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		month.ID, month.Month, month.OpeningBalanceCents, month.WalletBalanceCents, month.Status, month.CreatedAt, month.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create wallet month: %w", err)
	}
	return nil
}

func (r *Repository) Settings(ctx context.Context) (Settings, error) {
	allocationTemplates, err := r.ListAllocationTemplates(ctx, true)
	if err != nil {
		return Settings{}, err
	}
	incomeTemplates, err := r.ListIncomeTemplates(ctx, true)
	if err != nil {
		return Settings{}, err
	}
	categories, err := r.ListCategoriesAll(ctx, true)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		AllocationTemplates: allocationTemplates,
		IncomeTemplates:     incomeTemplates,
		Categories:          categories,
	}, nil
}

func (r *Repository) ListAllocationTemplates(ctx context.Context, includeInactive bool) ([]AllocationTemplate, error) {
	where := ""
	if !includeInactive {
		where = "WHERE active = 1"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, default_amount_cents, type, carry_forward,
			active, sort_order, created_at, updated_at
		FROM wallet_allocation_templates `+where+`
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list wallet allocation templates: %w", err)
	}
	defer rows.Close()

	var templates []AllocationTemplate
	for rows.Next() {
		template, err := scanAllocationTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if templates == nil {
		templates = []AllocationTemplate{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.attachDefaultCategoriesToTemplates(ctx, templates)
}

func (r *Repository) CreateAllocationTemplate(ctx context.Context, req CreateAllocationTemplateRequest) (AllocationTemplate, error) {
	template, err := sanitizeAllocationTemplate(req)
	if err != nil {
		return AllocationTemplate{}, err
	}
	categoryIDs, err := r.sanitizeDefaultCategoryIDs(ctx, req.DefaultCategoryIDs)
	if err != nil {
		return AllocationTemplate{}, err
	}
	now := shared.Now()
	template.ID = shared.NewID()
	template.CreatedAt = now
	template.UpdatedAt = now
	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_allocation_templates
			(id, name, default_amount_cents, type, carry_forward, active, sort_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			template.ID, template.Name, template.DefaultAmountCents, template.Type, boolInt(template.CarryForward),
			boolInt(template.Active), template.SortOrder, template.CreatedAt, template.UpdatedAt); err != nil {
			return fmt.Errorf("create wallet allocation template: %w", err)
		}
		return replaceTemplateDefaultCategories(ctx, tx, template.ID, categoryIDs, now)
	})
	if err != nil {
		return AllocationTemplate{}, err
	}
	return r.GetAllocationTemplate(ctx, template.ID)
}

func (r *Repository) GetAllocationTemplate(ctx context.Context, id string) (AllocationTemplate, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, default_amount_cents, type, carry_forward,
			active, sort_order, created_at, updated_at
		FROM wallet_allocation_templates WHERE id = ?`, id)
	template, err := scanAllocationTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AllocationTemplate{}, shared.ErrNotFound
	}
	if err != nil {
		return AllocationTemplate{}, err
	}
	templates, err := r.attachDefaultCategoriesToTemplates(ctx, []AllocationTemplate{template})
	if err != nil {
		return AllocationTemplate{}, err
	}
	return templates[0], nil
}

func (r *Repository) UpdateAllocationTemplate(ctx context.Context, id string, patch map[string]json.RawMessage) (AllocationTemplate, error) {
	current, err := r.GetAllocationTemplate(ctx, id)
	if err != nil {
		return AllocationTemplate{}, err
	}
	if raw, ok := patch["name"]; ok {
		current.Name, err = shared.ParseRequiredString(raw)
		if err != nil || current.Name == "" {
			return AllocationTemplate{}, errors.New("Allocation template name is required")
		}
	}
	if raw, ok := patch["default_amount_cents"]; ok {
		current.DefaultAmountCents, err = parsePatchInt64(raw, "default_amount_cents")
		if err != nil {
			return AllocationTemplate{}, err
		}
		if current.DefaultAmountCents < 0 {
			return AllocationTemplate{}, errors.New("default amount must be zero or greater")
		}
	}
	if raw, ok := patch["type"]; ok {
		value, err := shared.ParseRequiredString(raw)
		if err != nil {
			return AllocationTemplate{}, err
		}
		current.Type = normalizeAllocationType(value)
	}
	if raw, ok := patch["carry_forward"]; ok {
		current.CarryForward, err = parsePatchBool(raw, "carry_forward")
		if err != nil {
			return AllocationTemplate{}, err
		}
	}
	if raw, ok := patch["active"]; ok {
		current.Active, err = parsePatchBool(raw, "active")
		if err != nil {
			return AllocationTemplate{}, err
		}
	}
	if raw, ok := patch["sort_order"]; ok {
		current.SortOrder, err = parsePatchInt(raw, "sort_order")
		if err != nil {
			return AllocationTemplate{}, err
		}
	}
	var categoryIDs []string
	updateCategories := false
	if raw, ok := patch["default_category_ids"]; ok {
		if err := json.Unmarshal(raw, &categoryIDs); err != nil {
			return AllocationTemplate{}, errors.New("default_category_ids must be a list")
		}
		categoryIDs, err = r.sanitizeDefaultCategoryIDs(ctx, categoryIDs)
		if err != nil {
			return AllocationTemplate{}, err
		}
		updateCategories = true
	}
	current.UpdatedAt = shared.Now()
	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE wallet_allocation_templates
			SET name = ?, default_amount_cents = ?, type = ?, carry_forward = ?, active = ?, sort_order = ?, updated_at = ?
			WHERE id = ?`,
			current.Name, current.DefaultAmountCents, current.Type, boolInt(current.CarryForward), boolInt(current.Active),
			current.SortOrder, current.UpdatedAt, id)
		if err != nil {
			return fmt.Errorf("update wallet allocation template: %w", err)
		}
		if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
			return shared.ErrNotFound
		}
		if updateCategories {
			return replaceTemplateDefaultCategories(ctx, tx, id, categoryIDs, current.UpdatedAt)
		}
		return nil
	})
	if err != nil {
		return AllocationTemplate{}, err
	}
	return r.GetAllocationTemplate(ctx, id)
}

func (r *Repository) DeleteAllocationTemplate(ctx context.Context, id string) error {
	return deleteByID(ctx, r.db, "wallet_allocation_templates", id)
}

func (r *Repository) attachDefaultCategoriesToTemplates(ctx context.Context, templates []AllocationTemplate) ([]AllocationTemplate, error) {
	return attachTemplateDefaultCategories(ctx, r.db, templates)
}

func attachTemplateDefaultCategories(ctx context.Context, q shared.SQLer, templates []AllocationTemplate) ([]AllocationTemplate, error) {
	if len(templates) == 0 {
		return templates, nil
	}
	byID := make(map[string]int, len(templates))
	ids := make([]string, 0, len(templates))
	for i := range templates {
		templates[i].DefaultCategories = []Category{}
		byID[templates[i].ID] = i
		ids = append(ids, templates[i].ID)
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := q.QueryContext(ctx, `SELECT tc.template_id, c.id, c.name, c.system_key, c.active, c.sort_order, c.created_at, c.updated_at
		FROM wallet_allocation_template_categories tc
		JOIN wallet_categories c ON c.id = tc.category_id
		WHERE tc.template_id IN (`+placeholders+`)
		ORDER BY tc.sort_order ASC, c.sort_order ASC, c.name ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list wallet allocation template default categories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var templateID string
		var category Category
		var systemKey sql.NullString
		var active int
		if err := rows.Scan(&templateID, &category.ID, &category.Name, &systemKey, &active, &category.SortOrder, &category.CreatedAt, &category.UpdatedAt); err != nil {
			return nil, err
		}
		category.SystemKey = shared.FromNullString(systemKey)
		category.Active = intBool(active)
		if index, ok := byID[templateID]; ok {
			templates[index].DefaultCategories = append(templates[index].DefaultCategories, category)
		}
	}
	return templates, rows.Err()
}

func (r *Repository) sanitizeDefaultCategoryIDs(ctx context.Context, ids []string) ([]string, error) {
	return sanitizeDefaultCategoryIDs(ctx, r.db, ids)
}

func sanitizeDefaultCategoryIDs(ctx context.Context, q shared.SQLer, ids []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		var count int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_categories WHERE id = ? AND active = 1`, id).Scan(&count); err != nil {
			return nil, fmt.Errorf("check wallet default category: %w", err)
		}
		if count == 0 {
			return nil, errors.New("default category does not exist")
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func replaceTemplateDefaultCategories(ctx context.Context, q shared.SQLer, templateID string, categoryIDs []string, now string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM wallet_allocation_template_categories WHERE template_id = ?`, templateID); err != nil {
		return fmt.Errorf("clear wallet allocation template default categories: %w", err)
	}
	for i, categoryID := range categoryIDs {
		if _, err := q.ExecContext(ctx, `INSERT INTO wallet_allocation_template_categories
			(template_id, category_id, sort_order, created_at)
			VALUES (?, ?, ?, ?)`, templateID, categoryID, (i+1)*10, now); err != nil {
			return fmt.Errorf("save wallet allocation template default category: %w", err)
		}
	}
	return nil
}

func (r *Repository) ListIncomeTemplates(ctx context.Context, includeInactive bool) ([]IncomeTemplate, error) {
	where := ""
	if !includeInactive {
		where = "WHERE active = 1"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, default_amount_cents, default_day,
			active, sort_order, created_at, updated_at
		FROM wallet_income_templates `+where+`
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list wallet income templates: %w", err)
	}
	defer rows.Close()

	var templates []IncomeTemplate
	for rows.Next() {
		template, err := scanIncomeTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if templates == nil {
		templates = []IncomeTemplate{}
	}
	return templates, rows.Err()
}

func (r *Repository) CreateIncomeTemplate(ctx context.Context, req CreateIncomeTemplateRequest) (IncomeTemplate, error) {
	template, err := sanitizeIncomeTemplate(req)
	if err != nil {
		return IncomeTemplate{}, err
	}
	now := shared.Now()
	template.ID = shared.NewID()
	template.CreatedAt = now
	template.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `INSERT INTO wallet_income_templates
		(id, name, default_amount_cents, default_day, active, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		template.ID, template.Name, template.DefaultAmountCents, nullInt(template.DefaultDay),
		boolInt(template.Active), template.SortOrder, template.CreatedAt, template.UpdatedAt)
	if err != nil {
		return IncomeTemplate{}, fmt.Errorf("create wallet income template: %w", err)
	}
	return r.GetIncomeTemplate(ctx, template.ID)
}

func (r *Repository) GetIncomeTemplate(ctx context.Context, id string) (IncomeTemplate, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, default_amount_cents, default_day,
			active, sort_order, created_at, updated_at
		FROM wallet_income_templates WHERE id = ?`, id)
	template, err := scanIncomeTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IncomeTemplate{}, shared.ErrNotFound
	}
	if err != nil {
		return IncomeTemplate{}, err
	}
	return template, nil
}

func (r *Repository) UpdateIncomeTemplate(ctx context.Context, id string, patch map[string]json.RawMessage) (IncomeTemplate, error) {
	current, err := r.GetIncomeTemplate(ctx, id)
	if err != nil {
		return IncomeTemplate{}, err
	}
	if raw, ok := patch["name"]; ok {
		current.Name, err = shared.ParseRequiredString(raw)
		if err != nil || current.Name == "" {
			return IncomeTemplate{}, errors.New("Income template name is required")
		}
	}
	if raw, ok := patch["default_amount_cents"]; ok {
		current.DefaultAmountCents, err = parsePatchInt64(raw, "default_amount_cents")
		if err != nil {
			return IncomeTemplate{}, err
		}
		if current.DefaultAmountCents < 0 {
			return IncomeTemplate{}, errors.New("default amount must be zero or greater")
		}
	}
	if raw, ok := patch["default_day"]; ok {
		current.DefaultDay, err = parsePatchOptionalInt(raw, "default_day")
		if err != nil {
			return IncomeTemplate{}, err
		}
		if err := validateDefaultDay(current.DefaultDay); err != nil {
			return IncomeTemplate{}, err
		}
	}
	if raw, ok := patch["active"]; ok {
		current.Active, err = parsePatchBool(raw, "active")
		if err != nil {
			return IncomeTemplate{}, err
		}
	}
	if raw, ok := patch["sort_order"]; ok {
		current.SortOrder, err = parsePatchInt(raw, "sort_order")
		if err != nil {
			return IncomeTemplate{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_income_templates
		SET name = ?, default_amount_cents = ?, default_day = ?, active = ?, sort_order = ?, updated_at = ?
		WHERE id = ?`,
		current.Name, current.DefaultAmountCents, nullInt(current.DefaultDay),
		boolInt(current.Active), current.SortOrder, current.UpdatedAt, id)
	if err != nil {
		return IncomeTemplate{}, fmt.Errorf("update wallet income template: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return IncomeTemplate{}, shared.ErrNotFound
	}
	return r.GetIncomeTemplate(ctx, id)
}

func (r *Repository) DeleteIncomeTemplate(ctx context.Context, id string) error {
	return deleteByID(ctx, r.db, "wallet_income_templates", id)
}

func (r *Repository) ListCategoriesAll(ctx context.Context, includeInactive bool) ([]Category, error) {
	where := ""
	if !includeInactive {
		where = "WHERE active = 1"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, system_key, active, sort_order, created_at, updated_at
		FROM wallet_categories `+where+`
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list wallet categories: %w", err)
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	if categories == nil {
		categories = []Category{}
	}
	return categories, rows.Err()
}

func (r *Repository) CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error) {
	category, err := sanitizeCategory(req)
	if err != nil {
		return Category{}, err
	}
	now := shared.Now()
	category.ID = shared.NewID()
	category.CreatedAt = now
	category.UpdatedAt = now
	_, err = r.db.ExecContext(ctx, `INSERT INTO wallet_categories
		(id, name, active, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		category.ID, category.Name, boolInt(category.Active), category.SortOrder, category.CreatedAt, category.UpdatedAt)
	if err != nil {
		return Category{}, fmt.Errorf("create wallet category: %w", err)
	}
	return r.GetCategory(ctx, category.ID)
}

func (r *Repository) GetCategory(ctx context.Context, id string) (Category, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, system_key, active, sort_order, created_at, updated_at
		FROM wallet_categories WHERE id = ?`, id)
	category, err := scanCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, shared.ErrNotFound
	}
	if err != nil {
		return Category{}, err
	}
	return category, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, id string, patch map[string]json.RawMessage) (Category, error) {
	current, err := r.GetCategory(ctx, id)
	if err != nil {
		return Category{}, err
	}
	if current.SystemKey != nil && *current.SystemKey == "unsorted" {
		if _, ok := patch["active"]; ok {
			return Category{}, errors.New("Unsorted category cannot be deactivated")
		}
	}
	if raw, ok := patch["name"]; ok {
		current.Name, err = shared.ParseRequiredString(raw)
		if err != nil || current.Name == "" {
			return Category{}, errors.New("Category name is required")
		}
	}
	if raw, ok := patch["active"]; ok {
		current.Active, err = parsePatchBool(raw, "active")
		if err != nil {
			return Category{}, err
		}
	}
	if raw, ok := patch["sort_order"]; ok {
		current.SortOrder, err = parsePatchInt(raw, "sort_order")
		if err != nil {
			return Category{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_categories
		SET name = ?, active = ?, sort_order = ?, updated_at = ?
		WHERE id = ?`,
		current.Name, boolInt(current.Active), current.SortOrder, current.UpdatedAt, id)
	if err != nil {
		return Category{}, fmt.Errorf("update wallet category: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Category{}, shared.ErrNotFound
	}
	return r.GetCategory(ctx, id)
}

func (r *Repository) DeleteCategory(ctx context.Context, id string) error {
	category, err := r.GetCategory(ctx, id)
	if err != nil {
		return err
	}
	if category.SystemKey != nil && *category.SystemKey == "unsorted" {
		return errors.New("Unsorted category cannot be deleted")
	}
	return deleteByID(ctx, r.db, "wallet_categories", id)
}

func listActiveAllocationTemplates(ctx context.Context, q shared.SQLer) ([]AllocationTemplate, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, default_amount_cents, type, carry_forward,
			active, sort_order, created_at, updated_at
		FROM wallet_allocation_templates
		WHERE active = 1
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list active wallet allocation templates: %w", err)
	}
	defer rows.Close()
	var templates []AllocationTemplate
	for rows.Next() {
		template, err := scanAllocationTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attachTemplateDefaultCategories(ctx, q, templates)
}

func listActiveIncomeTemplates(ctx context.Context, q shared.SQLer) ([]IncomeTemplate, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, default_amount_cents, default_day,
			active, sort_order, created_at, updated_at
		FROM wallet_income_templates
		WHERE active = 1
		ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list active wallet income templates: %w", err)
	}
	defer rows.Close()
	var templates []IncomeTemplate
	for rows.Next() {
		template, err := scanIncomeTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, rows.Err()
}

func previousWalletMonthID(ctx context.Context, q shared.SQLer, monthKey string) (*string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id FROM wallet_months
		WHERE month < ?
		ORDER BY month DESC
		LIMIT 1`, monthKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get previous wallet month: %w", err)
	}
	return &id, nil
}

func previousAllocationRemaining(ctx context.Context, q shared.SQLer, monthID string, template AllocationTemplate) (int64, error) {
	var remaining int64
	err := q.QueryRowContext(ctx, `SELECT
			a.budgeted_cents - COALESCE(SUM(CASE WHEN t.kind = 'spend' THEN t.amount_cents ELSE 0 END), 0) AS remaining_cents
		FROM wallet_allocations a
		LEFT JOIN wallet_transactions t ON t.allocation_id = a.id
		WHERE a.month_id = ? AND (a.template_id = ? OR (a.template_id IS NULL AND lower(a.name) = lower(?)))
		GROUP BY a.id
		ORDER BY CASE WHEN a.template_id = ? THEN 0 ELSE 1 END
		LIMIT 1`, monthID, template.ID, template.Name, template.ID).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get previous allocation remaining: %w", err)
	}
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

func templateReceivedDate(monthKey string, day *int) *string {
	if day == nil {
		return nil
	}
	monthStart, err := time.Parse("2006-01", monthKey)
	if err != nil {
		return nil
	}
	lastDay := time.Date(monthStart.Year(), monthStart.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	selectedDay := *day
	if selectedDay > lastDay {
		selectedDay = lastDay
	}
	date := time.Date(monthStart.Year(), monthStart.Month(), selectedDay, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	return &date
}

func sanitizeAllocationTemplate(req CreateAllocationTemplateRequest) (AllocationTemplate, error) {
	name, err := normalizeRequiredName(req.Name, "Allocation template name")
	if err != nil {
		return AllocationTemplate{}, err
	}
	if req.DefaultAmountCents < 0 {
		return AllocationTemplate{}, errors.New("default amount must be zero or greater")
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	return AllocationTemplate{
		Name:               name,
		DefaultAmountCents: req.DefaultAmountCents,
		Type:               normalizeAllocationType(req.Type),
		CarryForward:       req.CarryForward,
		Active:             active,
		SortOrder:          req.SortOrder,
	}, nil
}

func sanitizeIncomeTemplate(req CreateIncomeTemplateRequest) (IncomeTemplate, error) {
	name, err := normalizeRequiredName(req.Name, "Income template name")
	if err != nil {
		return IncomeTemplate{}, err
	}
	if req.DefaultAmountCents < 0 {
		return IncomeTemplate{}, errors.New("default amount must be zero or greater")
	}
	if err := validateDefaultDay(req.DefaultDay); err != nil {
		return IncomeTemplate{}, err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	return IncomeTemplate{
		Name:               name,
		DefaultAmountCents: req.DefaultAmountCents,
		DefaultDay:         req.DefaultDay,
		Active:             active,
		SortOrder:          req.SortOrder,
	}, nil
}

func sanitizeCategory(req CreateCategoryRequest) (Category, error) {
	name, err := normalizeRequiredName(req.Name, "Category name")
	if err != nil {
		return Category{}, err
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	return Category{
		Name:      name,
		Active:    active,
		SortOrder: req.SortOrder,
	}, nil
}

func validateDefaultDay(day *int) error {
	if day == nil {
		return nil
	}
	if *day < 1 || *day > 31 {
		return errors.New("default_day must be between 1 and 31")
	}
	return nil
}

func scanAllocationTemplate(scanner interface {
	Scan(dest ...any) error
}) (AllocationTemplate, error) {
	var template AllocationTemplate
	var carryForward, active int
	if err := scanner.Scan(&template.ID, &template.Name, &template.DefaultAmountCents, &template.Type,
		&carryForward, &active, &template.SortOrder, &template.CreatedAt, &template.UpdatedAt); err != nil {
		return AllocationTemplate{}, err
	}
	template.CarryForward = intBool(carryForward)
	template.Active = intBool(active)
	return template, nil
}

func scanIncomeTemplate(scanner interface {
	Scan(dest ...any) error
}) (IncomeTemplate, error) {
	var template IncomeTemplate
	var day sql.NullInt64
	var active int
	if err := scanner.Scan(&template.ID, &template.Name, &template.DefaultAmountCents, &day,
		&active, &template.SortOrder, &template.CreatedAt, &template.UpdatedAt); err != nil {
		return IncomeTemplate{}, err
	}
	if day.Valid {
		value := int(day.Int64)
		template.DefaultDay = &value
	}
	template.Active = intBool(active)
	return template, nil
}

func nullInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}
