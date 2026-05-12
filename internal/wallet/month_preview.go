package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"private-workspace/internal/shared"
)

func (r *Repository) PreviewMonth(ctx context.Context, req CreateMonthRequest) (MonthPreview, error) {
	month, err := r.previewMonthShell(ctx, req)
	if err != nil {
		return MonthPreview{}, err
	}
	if err := r.ensureMonthDoesNotExist(ctx, month.Month); err != nil {
		return MonthPreview{}, err
	}

	incomeItems, allocations, err := r.preparedMonthRows(ctx, r.db, month, req)
	if err != nil {
		return MonthPreview{}, err
	}
	applyInitialWalletBalance(&month, req, incomeItems)
	categories, err := r.ListCategories(ctx)
	if err != nil {
		return MonthPreview{}, err
	}
	return MonthPreview{
		Month:       month,
		IncomeItems: incomeItems,
		Allocations: allocations,
		Categories:  categories,
		Source:      "preview",
	}, nil
}

func (r *Repository) preparedMonthRows(ctx context.Context, q shared.SQLer, month Month, req CreateMonthRequest) ([]IncomeItem, []Allocation, error) {
	if len(req.IncomeItems) > 0 || len(req.Allocations) > 0 {
		return r.reviewedPreviewRows(ctx, q, month, req)
	}
	if req.UseTemplates {
		return r.templatePreviewRows(ctx, q, month, req.CarryForward)
	}
	return []IncomeItem{}, []Allocation{}, nil
}

func applyInitialWalletBalance(month *Month, req CreateMonthRequest, incomeItems []IncomeItem) {
	if req.WalletBalanceCents != nil {
		month.WalletBalanceCents = *req.WalletBalanceCents
		return
	}
	incomeTotal := int64(0)
	for _, item := range incomeItems {
		incomeTotal += item.AmountCents
	}
	month.WalletBalanceCents = month.OpeningBalanceCents + incomeTotal
}

func insertPreparedMonthRows(ctx context.Context, tx *sql.Tx, incomeItems []IncomeItem, allocations []Allocation) error {
	for _, item := range incomeItems {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_income_items
			(id, month_id, name, amount_cents, received_date, applies_to_month, notes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.MonthID, item.Name, item.AmountCents, shared.NullString(item.ReceivedDate),
			item.AppliesToMonth, shared.NullString(item.Notes), item.CreatedAt, item.UpdatedAt); err != nil {
			return fmt.Errorf("create reviewed wallet income: %w", err)
		}
	}
	for _, allocation := range allocations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_allocations
			(id, month_id, template_id, name, budgeted_cents, type, carry_forward, sort_order, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			allocation.ID, allocation.MonthID, shared.NullString(allocation.TemplateID), allocation.Name,
			allocation.BudgetedCents, allocation.Type, boolInt(allocation.CarryForward), allocation.SortOrder,
			boolInt(allocation.Active), allocation.CreatedAt, allocation.UpdatedAt); err != nil {
			return fmt.Errorf("create reviewed wallet allocation: %w", err)
		}
		if err := insertAllocationDefaultCategories(ctx, tx, allocation.ID, categoryIDs(allocation.DefaultCategories), allocation.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) previewMonthShell(ctx context.Context, req CreateMonthRequest) (Month, error) {
	monthKey, err := validateMonth(req.Month)
	if err != nil {
		return Month{}, err
	}
	walletBalance := req.OpeningBalanceCents
	if req.WalletBalanceCents != nil {
		walletBalance = *req.WalletBalanceCents
	}
	now := shared.Now()
	return Month{
		ID:                  shared.NewID(),
		Month:               monthKey,
		OpeningBalanceCents: req.OpeningBalanceCents,
		WalletBalanceCents:  walletBalance,
		Status:              "open",
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (r *Repository) ensureMonthDoesNotExist(ctx context.Context, monthKey string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_months WHERE month = ?`, monthKey).Scan(&count); err != nil {
		return fmt.Errorf("check wallet month: %w", err)
	}
	if count > 0 {
		return errors.New("wallet month already exists")
	}
	return nil
}

func (r *Repository) templatePreviewRows(ctx context.Context, q shared.SQLer, month Month, carryForward bool) ([]IncomeItem, []Allocation, error) {
	templates, err := listActiveAllocationTemplates(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	previousMonthID, err := previousWalletMonthID(ctx, q, month.Month)
	if err != nil {
		return nil, nil, err
	}
	allocations := make([]Allocation, 0, len(templates))
	for _, template := range templates {
		amount := template.DefaultAmountCents
		if carryForward && template.CarryForward && previousMonthID != nil {
			remaining, err := previousAllocationRemaining(ctx, q, *previousMonthID, template)
			if err != nil {
				return nil, nil, err
			}
			if remaining > 0 {
				amount += remaining
			}
		}
		templateID := template.ID
		allocations = append(allocations, Allocation{
			ID:                shared.NewID(),
			MonthID:           month.ID,
			TemplateID:        &templateID,
			Name:              template.Name,
			BudgetedCents:     amount,
			Type:              template.Type,
			CarryForward:      template.CarryForward,
			SortOrder:         template.SortOrder,
			Active:            true,
			DefaultCategories: append([]Category{}, template.DefaultCategories...),
			CreatedAt:         month.CreatedAt,
			UpdatedAt:         month.UpdatedAt,
		})
	}

	incomeTemplates, err := listActiveIncomeTemplates(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	incomeItems := make([]IncomeItem, 0, len(incomeTemplates))
	for _, template := range incomeTemplates {
		incomeItems = append(incomeItems, IncomeItem{
			ID:             shared.NewID(),
			MonthID:        month.ID,
			Name:           template.Name,
			AmountCents:    template.DefaultAmountCents,
			ReceivedDate:   templateReceivedDate(month.Month, template.DefaultDay),
			AppliesToMonth: month.Month,
			CreatedAt:      month.CreatedAt,
			UpdatedAt:      month.UpdatedAt,
		})
	}
	return incomeItems, allocations, nil
}

func (r *Repository) reviewedPreviewRows(ctx context.Context, q shared.SQLer, month Month, req CreateMonthRequest) ([]IncomeItem, []Allocation, error) {
	incomeItems := make([]IncomeItem, 0, len(req.IncomeItems))
	for _, input := range req.IncomeItems {
		item, err := sanitizePreviewIncome(month, input)
		if err != nil {
			return nil, nil, err
		}
		incomeItems = append(incomeItems, item)
	}
	allocations := make([]Allocation, 0, len(req.Allocations))
	for _, input := range req.Allocations {
		allocation, err := sanitizePreviewAllocation(ctx, q, month, input)
		if err != nil {
			return nil, nil, err
		}
		allocations = append(allocations, allocation)
	}
	return incomeItems, allocations, nil
}

func sanitizePreviewIncome(month Month, input MonthPreviewIncomeItemRequest) (IncomeItem, error) {
	name, err := normalizeRequiredName(input.Name, "Income name")
	if err != nil {
		return IncomeItem{}, err
	}
	if input.AmountCents < 0 {
		return IncomeItem{}, errors.New("income amount must be zero or greater")
	}
	receivedDate := optionalStringPtr(input.ReceivedDate)
	if receivedDate != nil {
		date, err := validateDate(*receivedDate, "received_date")
		if err != nil {
			return IncomeItem{}, err
		}
		receivedDate = &date
	}
	appliesToMonth := strings.TrimSpace(input.AppliesToMonth)
	if appliesToMonth == "" {
		appliesToMonth = month.Month
	}
	appliesToMonth, err = validateMonth(appliesToMonth)
	if err != nil {
		return IncomeItem{}, err
	}
	return IncomeItem{
		ID:             shared.NewID(),
		MonthID:        month.ID,
		Name:           name,
		AmountCents:    input.AmountCents,
		ReceivedDate:   receivedDate,
		AppliesToMonth: appliesToMonth,
		Notes:          optionalStringPtr(input.Notes),
		CreatedAt:      month.CreatedAt,
		UpdatedAt:      month.UpdatedAt,
	}, nil
}

func sanitizePreviewAllocation(ctx context.Context, q shared.SQLer, month Month, input MonthPreviewAllocationRequest) (Allocation, error) {
	name, err := normalizeRequiredName(input.Name, "Allocation name")
	if err != nil {
		return Allocation{}, err
	}
	if input.BudgetedCents < 0 {
		return Allocation{}, errors.New("allocation budget must be zero or greater")
	}
	var templateID *string
	if input.TemplateID != nil {
		trimmed := strings.TrimSpace(*input.TemplateID)
		if trimmed != "" {
			var count int
			if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_allocation_templates WHERE id = ?`, trimmed).Scan(&count); err != nil {
				return Allocation{}, fmt.Errorf("check wallet allocation template: %w", err)
			}
			if count == 0 {
				return Allocation{}, errors.New("allocation template does not exist")
			}
			templateID = &trimmed
		}
	}
	categoryIDs, err := sanitizeDefaultCategoryIDs(ctx, q, input.DefaultCategoryIDs)
	if err != nil {
		return Allocation{}, err
	}
	categories, err := categoriesByID(ctx, q, categoryIDs)
	if err != nil {
		return Allocation{}, err
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	return Allocation{
		ID:                shared.NewID(),
		MonthID:           month.ID,
		TemplateID:        templateID,
		Name:              name,
		BudgetedCents:     input.BudgetedCents,
		Type:              normalizeAllocationType(input.Type),
		CarryForward:      input.CarryForward,
		SortOrder:         input.SortOrder,
		Active:            active,
		DefaultCategories: categories,
		CreatedAt:         month.CreatedAt,
		UpdatedAt:         month.UpdatedAt,
	}, nil
}

func categoriesByID(ctx context.Context, q shared.SQLer, ids []string) ([]Category, error) {
	if len(ids) == 0 {
		return []Category{}, nil
	}
	categories := make([]Category, 0, len(ids))
	for _, id := range ids {
		row := q.QueryRowContext(ctx, `SELECT id, name, system_key, active, sort_order, created_at, updated_at
			FROM wallet_categories
			WHERE id = ?`, id)
		category, err := scanCategory(row)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, nil
}
