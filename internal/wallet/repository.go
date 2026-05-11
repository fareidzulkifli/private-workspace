package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"private-workspace/internal/db"
	"private-workspace/internal/shared"
)

type Repository struct {
	db *db.DB
}

func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

func (r *Repository) ListMonths(ctx context.Context) ([]Month, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, month, opening_balance_cents, wallet_balance_cents,
			status, closed_at, closed_wallet_balance_cents, created_at, updated_at
		FROM wallet_months
		ORDER BY month DESC`)
	if err != nil {
		return nil, fmt.Errorf("list wallet months: %w", err)
	}
	defer rows.Close()

	var months []Month
	for rows.Next() {
		month, err := scanMonth(rows)
		if err != nil {
			return nil, err
		}
		months = append(months, month)
	}
	if months == nil {
		months = []Month{}
	}
	return months, rows.Err()
}

func (r *Repository) ListMonthBook(ctx context.Context) ([]MonthBookRow, error) {
	months, err := r.ListMonths(ctx)
	if err != nil {
		return nil, err
	}
	book := make([]MonthBookRow, 0, len(months))
	for _, month := range months {
		_, incomeTotal, err := r.listIncomeForMonth(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		allocations, totalReserved, spendingTotal, err := r.listAllocationSummaries(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		adjustmentTotal, err := r.adjustmentTotal(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		transactionCount, err := r.visibleTransactionCount(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		expectedBalance := month.OpeningBalanceCents + incomeTotal - spendingTotal + adjustmentTotal
		book = append(book, MonthBookRow{
			ID:                       month.ID,
			Month:                    month.Month,
			Status:                   month.Status,
			OpeningBalanceCents:      month.OpeningBalanceCents,
			WalletBalanceCents:       month.WalletBalanceCents,
			ClosedAt:                 month.ClosedAt,
			ClosedWalletBalanceCents: month.ClosedWalletBalanceCents,
			CreatedAt:                month.CreatedAt,
			UpdatedAt:                month.UpdatedAt,
			IncomeTotalCents:         incomeTotal,
			SpendingTotalCents:       spendingTotal,
			AdjustmentTotalCents:     adjustmentTotal,
			ExpectedBalanceCents:     expectedBalance,
			VarianceCents:            month.WalletBalanceCents - expectedBalance,
			TotalReservedCents:       totalReserved,
			AvailableBalanceCents:    month.WalletBalanceCents - totalReserved,
			AllocationCount:          len(allocations),
			TransactionCount:         transactionCount,
		})
	}
	return book, nil
}

func (r *Repository) CreateMonth(ctx context.Context, req CreateMonthRequest) (Month, error) {
	monthKey, err := validateMonth(req.Month)
	if err != nil {
		return Month{}, err
	}
	walletBalance := req.OpeningBalanceCents
	if req.WalletBalanceCents != nil {
		walletBalance = *req.WalletBalanceCents
	}
	now := shared.Now()
	month := Month{
		ID:                  shared.NewID(),
		Month:               monthKey,
		OpeningBalanceCents: req.OpeningBalanceCents,
		WalletBalanceCents:  walletBalance,
		Status:              "open",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if len(req.IncomeItems) > 0 || len(req.Allocations) > 0 {
		err = r.createMonthFromReviewedRows(ctx, month, req)
	} else if req.UseTemplates {
		err = r.db.Tx(ctx, func(tx *sql.Tx) error {
			if err := insertMonth(ctx, tx, month); err != nil {
				return err
			}
			if err := r.populateMonthFromTemplates(ctx, tx, month, req.CarryForward); err != nil {
				return err
			}
			return nil
		})
	} else {
		err = insertMonth(ctx, r.db, month)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Month{}, errors.New("wallet month already exists")
		}
		return Month{}, err
	}
	return month, nil
}

func (r *Repository) GetMonth(ctx context.Context, monthKey string) (Month, error) {
	monthKey, err := validateMonth(monthKey)
	if err != nil {
		return Month{}, err
	}
	row := r.db.QueryRowContext(ctx, `SELECT id, month, opening_balance_cents, wallet_balance_cents,
			status, closed_at, closed_wallet_balance_cents, created_at, updated_at
		FROM wallet_months
		WHERE month = ?`, monthKey)
	month, err := scanMonth(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Month{}, shared.ErrNotFound
	}
	if err != nil {
		return Month{}, err
	}
	return month, nil
}

func (r *Repository) UpdateMonth(ctx context.Context, monthKey string, patch map[string]json.RawMessage) (Month, error) {
	current, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return Month{}, err
	}
	if current.Status == "closed" {
		return Month{}, errors.New("wallet month is closed")
	}
	if raw, ok := patch["opening_balance_cents"]; ok {
		current.OpeningBalanceCents, err = parsePatchInt64(raw, "opening_balance_cents")
		if err != nil {
			return Month{}, err
		}
	}
	if raw, ok := patch["wallet_balance_cents"]; ok {
		current.WalletBalanceCents, err = parsePatchInt64(raw, "wallet_balance_cents")
		if err != nil {
			return Month{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_months
		SET opening_balance_cents = ?, wallet_balance_cents = ?, updated_at = ?
		WHERE id = ?`,
		current.OpeningBalanceCents, current.WalletBalanceCents, current.UpdatedAt, current.ID)
	if err != nil {
		return Month{}, fmt.Errorf("update wallet month: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Month{}, shared.ErrNotFound
	}
	return r.GetMonth(ctx, current.Month)
}

func (r *Repository) DeleteMonth(ctx context.Context, monthKey string) error {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM wallet_months WHERE id = ?`, month.ID)
	if err != nil {
		return fmt.Errorf("delete wallet month: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func (r *Repository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, system_key, active, sort_order, created_at, updated_at
		FROM wallet_categories
		WHERE active = 1
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

func (r *Repository) CreateIncome(ctx context.Context, monthKey string, req CreateIncomeRequest) (IncomeItem, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return IncomeItem{}, err
	}
	if month.Status == "closed" {
		return IncomeItem{}, errors.New("wallet month is closed")
	}
	name, err := normalizeRequiredName(req.Name, "Income name")
	if err != nil {
		return IncomeItem{}, err
	}
	if req.AmountCents < 0 {
		return IncomeItem{}, errors.New("income amount must be zero or greater")
	}
	receivedDate := optionalStringPtr(req.ReceivedDate)
	if receivedDate != nil {
		date, err := validateDate(*receivedDate, "received_date")
		if err != nil {
			return IncomeItem{}, err
		}
		receivedDate = &date
	}
	appliesToMonth := strings.TrimSpace(req.AppliesToMonth)
	if appliesToMonth == "" {
		appliesToMonth = month.Month
	}
	appliesToMonth, err = validateMonth(appliesToMonth)
	if err != nil {
		return IncomeItem{}, err
	}
	now := shared.Now()
	id := shared.NewID()
	_, err = r.db.ExecContext(ctx, `INSERT INTO wallet_income_items
		(id, month_id, name, amount_cents, received_date, applies_to_month, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, month.ID, name, req.AmountCents, shared.NullString(receivedDate), appliesToMonth, normalizeOptionalString(req.Notes), now, now)
	if err != nil {
		return IncomeItem{}, fmt.Errorf("create wallet income: %w", err)
	}
	return r.GetIncome(ctx, id)
}

func (r *Repository) GetIncome(ctx context.Context, id string) (IncomeItem, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, month_id, name, amount_cents, received_date,
			applies_to_month, notes, created_at, updated_at
		FROM wallet_income_items WHERE id = ?`, id)
	item, err := scanIncome(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IncomeItem{}, shared.ErrNotFound
	}
	if err != nil {
		return IncomeItem{}, err
	}
	return item, nil
}

func (r *Repository) UpdateIncome(ctx context.Context, id string, patch map[string]json.RawMessage) (IncomeItem, error) {
	current, err := r.GetIncome(ctx, id)
	if err != nil {
		return IncomeItem{}, err
	}
	if err := r.ensureMonthOpen(ctx, current.MonthID); err != nil {
		return IncomeItem{}, err
	}
	if raw, ok := patch["name"]; ok {
		current.Name, err = shared.ParseRequiredString(raw)
		if err != nil || current.Name == "" {
			return IncomeItem{}, errors.New("Income name is required")
		}
	}
	if raw, ok := patch["amount_cents"]; ok {
		current.AmountCents, err = parsePatchInt64(raw, "amount_cents")
		if err != nil {
			return IncomeItem{}, err
		}
		if current.AmountCents < 0 {
			return IncomeItem{}, errors.New("income amount must be zero or greater")
		}
	}
	if raw, ok := patch["received_date"]; ok {
		current.ReceivedDate, err = parsePatchOptionalString(raw, "received_date")
		if err != nil {
			return IncomeItem{}, err
		}
		if current.ReceivedDate != nil {
			date, err := validateDate(*current.ReceivedDate, "received_date")
			if err != nil {
				return IncomeItem{}, err
			}
			current.ReceivedDate = &date
		}
	}
	if raw, ok := patch["applies_to_month"]; ok {
		current.AppliesToMonth, err = shared.ParseRequiredString(raw)
		if err != nil {
			return IncomeItem{}, err
		}
		current.AppliesToMonth, err = validateMonth(current.AppliesToMonth)
		if err != nil {
			return IncomeItem{}, err
		}
	}
	if raw, ok := patch["notes"]; ok {
		current.Notes, err = parsePatchOptionalString(raw, "notes")
		if err != nil {
			return IncomeItem{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_income_items
		SET name = ?, amount_cents = ?, received_date = ?, applies_to_month = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		current.Name, current.AmountCents, shared.NullString(current.ReceivedDate), current.AppliesToMonth,
		shared.NullString(current.Notes), current.UpdatedAt, id)
	if err != nil {
		return IncomeItem{}, fmt.Errorf("update wallet income: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return IncomeItem{}, shared.ErrNotFound
	}
	return r.GetIncome(ctx, id)
}

func (r *Repository) DeleteIncome(ctx context.Context, id string) error {
	item, err := r.GetIncome(ctx, id)
	if err != nil {
		return err
	}
	if err := r.ensureMonthOpen(ctx, item.MonthID); err != nil {
		return err
	}
	return deleteByID(ctx, r.db, "wallet_income_items", id)
}

func (r *Repository) CreateAllocation(ctx context.Context, monthKey string, req CreateAllocationRequest) (Allocation, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return Allocation{}, err
	}
	if month.Status == "closed" {
		return Allocation{}, errors.New("wallet month is closed")
	}
	name, err := normalizeRequiredName(req.Name, "Allocation name")
	if err != nil {
		return Allocation{}, err
	}
	if req.BudgetedCents < 0 {
		return Allocation{}, errors.New("allocation budget must be zero or greater")
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	allocationType := normalizeAllocationType(req.Type)
	templateID := optionalStringPtr(req.TemplateID)
	if templateID != nil {
		if err := r.ensureAllocationTemplateExists(ctx, *templateID); err != nil {
			return Allocation{}, err
		}
	}
	categoryIDs, err := r.sanitizeDefaultCategoryIDs(ctx, req.DefaultCategoryIDs)
	if err != nil {
		return Allocation{}, err
	}
	now := shared.Now()
	id := shared.NewID()
	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_allocations
			(id, month_id, template_id, name, budgeted_cents, type, carry_forward, sort_order, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, month.ID, shared.NullString(templateID), name, req.BudgetedCents, allocationType, boolInt(req.CarryForward),
			req.SortOrder, boolInt(active), now, now); err != nil {
			return fmt.Errorf("create wallet allocation: %w", err)
		}
		return insertAllocationDefaultCategories(ctx, tx, id, categoryIDs, now)
	})
	if err != nil {
		return Allocation{}, err
	}
	return r.GetAllocation(ctx, id)
}

func (r *Repository) GetAllocation(ctx context.Context, id string) (Allocation, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, month_id, template_id, name, budgeted_cents,
			type, carry_forward, sort_order, active, created_at, updated_at
		FROM wallet_allocations WHERE id = ?`, id)
	allocation, err := scanAllocation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Allocation{}, shared.ErrNotFound
	}
	if err != nil {
		return Allocation{}, err
	}
	allocation.DefaultCategories, err = r.defaultCategoriesForAllocation(ctx, allocation)
	if err != nil {
		return Allocation{}, err
	}
	return allocation, nil
}

func (r *Repository) UpdateAllocation(ctx context.Context, id string, patch map[string]json.RawMessage) (Allocation, error) {
	current, err := r.GetAllocation(ctx, id)
	if err != nil {
		return Allocation{}, err
	}
	if err := r.ensureMonthOpen(ctx, current.MonthID); err != nil {
		return Allocation{}, err
	}
	if raw, ok := patch["name"]; ok {
		current.Name, err = shared.ParseRequiredString(raw)
		if err != nil || current.Name == "" {
			return Allocation{}, errors.New("Allocation name is required")
		}
	}
	if raw, ok := patch["budgeted_cents"]; ok {
		current.BudgetedCents, err = parsePatchInt64(raw, "budgeted_cents")
		if err != nil {
			return Allocation{}, err
		}
		if current.BudgetedCents < 0 {
			return Allocation{}, errors.New("allocation budget must be zero or greater")
		}
	}
	if raw, ok := patch["type"]; ok {
		value, err := shared.ParseRequiredString(raw)
		if err != nil {
			return Allocation{}, err
		}
		current.Type = normalizeAllocationType(value)
	}
	if raw, ok := patch["carry_forward"]; ok {
		current.CarryForward, err = parsePatchBool(raw, "carry_forward")
		if err != nil {
			return Allocation{}, err
		}
	}
	if raw, ok := patch["sort_order"]; ok {
		current.SortOrder, err = parsePatchInt(raw, "sort_order")
		if err != nil {
			return Allocation{}, err
		}
	}
	if raw, ok := patch["active"]; ok {
		current.Active, err = parsePatchBool(raw, "active")
		if err != nil {
			return Allocation{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_allocations
		SET name = ?, budgeted_cents = ?, type = ?, carry_forward = ?, sort_order = ?, active = ?, updated_at = ?
		WHERE id = ?`,
		current.Name, current.BudgetedCents, current.Type, boolInt(current.CarryForward), current.SortOrder,
		boolInt(current.Active), current.UpdatedAt, id)
	if err != nil {
		return Allocation{}, fmt.Errorf("update wallet allocation: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Allocation{}, shared.ErrNotFound
	}
	return r.GetAllocation(ctx, id)
}

func (r *Repository) DeleteAllocation(ctx context.Context, id string) error {
	allocation, err := r.GetAllocation(ctx, id)
	if err != nil {
		return err
	}
	if err := r.ensureMonthOpen(ctx, allocation.MonthID); err != nil {
		return err
	}
	return deleteByID(ctx, r.db, "wallet_allocations", id)
}

func (r *Repository) CreateTransaction(ctx context.Context, monthKey string, req CreateTransactionRequest) (Transaction, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return Transaction{}, err
	}
	if month.Status == "closed" {
		return Transaction{}, errors.New("wallet month is closed")
	}
	if err := r.ensureAllocationBelongsToMonth(ctx, req.AllocationID, month.ID); err != nil {
		return Transaction{}, err
	}
	categoryID := strings.TrimSpace(req.CategoryID)
	if categoryID == "" {
		categoryID = UnsortedCategoryID
	}
	if err := r.ensureCategoryExists(ctx, categoryID); err != nil {
		return Transaction{}, err
	}
	if req.AmountCents <= 0 {
		return Transaction{}, errors.New("transaction amount must be greater than zero")
	}
	date := strings.TrimSpace(req.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	date, err = validateDate(date, "date")
	if err != nil {
		return Transaction{}, err
	}
	now := shared.Now()
	id := shared.NewID()
	_, err = r.db.ExecContext(ctx, `INSERT INTO wallet_transactions
		(id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'spend', 'manual', ?, ?)`,
		id, month.ID, strings.TrimSpace(req.AllocationID), categoryID, date, req.AmountCents,
		normalizeOptionalString(req.Note), boolInt(req.Rounded), now, now)
	if err != nil {
		return Transaction{}, fmt.Errorf("create wallet transaction: %w", err)
	}
	return r.GetTransaction(ctx, id)
}

func (r *Repository) GetTransaction(ctx context.Context, id string) (Transaction, error) {
	row := r.db.QueryRowContext(ctx, transactionSelectSQL()+` WHERE t.id = ?`, id)
	transaction, err := scanTransaction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Transaction{}, shared.ErrNotFound
	}
	if err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}

func (r *Repository) UpdateTransaction(ctx context.Context, id string, patch map[string]json.RawMessage) (Transaction, error) {
	current, err := r.GetTransaction(ctx, id)
	if err != nil {
		return Transaction{}, err
	}
	if err := r.ensureMonthOpen(ctx, current.MonthID); err != nil {
		return Transaction{}, err
	}
	if current.ParentTransactionID != nil {
		if _, amountPatch := patch["amount_cents"]; amountPatch {
			return Transaction{}, errors.New("split detail amount cannot be edited")
		}
	}
	if raw, ok := patch["allocation_id"]; ok {
		current.AllocationID, err = shared.ParseRequiredString(raw)
		if err != nil || current.AllocationID == "" {
			return Transaction{}, errors.New("allocation_id is required")
		}
		if err := r.ensureAllocationBelongsToMonth(ctx, current.AllocationID, current.MonthID); err != nil {
			return Transaction{}, err
		}
	}
	if raw, ok := patch["category_id"]; ok {
		current.CategoryID, err = shared.ParseRequiredString(raw)
		if err != nil || current.CategoryID == "" {
			return Transaction{}, errors.New("category_id is required")
		}
		if err := r.ensureCategoryExists(ctx, current.CategoryID); err != nil {
			return Transaction{}, err
		}
	}
	if raw, ok := patch["date"]; ok {
		current.Date, err = shared.ParseRequiredString(raw)
		if err != nil {
			return Transaction{}, err
		}
		current.Date, err = validateDate(current.Date, "date")
		if err != nil {
			return Transaction{}, err
		}
	}
	if raw, ok := patch["amount_cents"]; ok {
		current.AmountCents, err = parsePatchInt64(raw, "amount_cents")
		if err != nil {
			return Transaction{}, err
		}
		if current.AmountCents <= 0 {
			return Transaction{}, errors.New("transaction amount must be greater than zero")
		}
	}
	if raw, ok := patch["note"]; ok {
		current.Note, err = parsePatchOptionalString(raw, "note")
		if err != nil {
			return Transaction{}, err
		}
	}
	if raw, ok := patch["rounded"]; ok {
		current.Rounded, err = parsePatchBool(raw, "rounded")
		if err != nil {
			return Transaction{}, err
		}
	}
	current.UpdatedAt = shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_transactions
		SET allocation_id = ?, category_id = ?, date = ?, amount_cents = ?, note = ?, rounded = ?, updated_at = ?
		WHERE id = ?`,
		current.AllocationID, current.CategoryID, current.Date, current.AmountCents, shared.NullString(current.Note),
		boolInt(current.Rounded), current.UpdatedAt, id)
	if err != nil {
		return Transaction{}, fmt.Errorf("update wallet transaction: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Transaction{}, shared.ErrNotFound
	}
	return r.GetTransaction(ctx, id)
}

func (r *Repository) DeleteTransaction(ctx context.Context, id string) error {
	current, err := r.GetTransaction(ctx, id)
	if err != nil {
		return err
	}
	if err := r.ensureMonthOpen(ctx, current.MonthID); err != nil {
		return err
	}
	if current.ParentTransactionID != nil {
		return errors.New("split detail cannot be deleted directly")
	}
	return r.db.Tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT child_transaction_id
			FROM wallet_transaction_splits
			WHERE parent_transaction_id = ?`, id)
		if err != nil {
			return fmt.Errorf("list split children: %w", err)
		}
		defer rows.Close()
		var childIDs []string
		for rows.Next() {
			var childID string
			if err := rows.Scan(&childID); err != nil {
				return err
			}
			childIDs = append(childIDs, childID)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, childID := range childIDs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM wallet_transactions WHERE id = ?`, childID); err != nil {
				return fmt.Errorf("delete split child: %w", err)
			}
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM wallet_transactions WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete wallet_transactions: %w", err)
		}
		if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
			return shared.ErrNotFound
		}
		return nil
	})
}

func (r *Repository) ensureAllocationBelongsToMonth(ctx context.Context, allocationID string, monthID string) error {
	allocationID = strings.TrimSpace(allocationID)
	if allocationID == "" {
		return errors.New("allocation_id is required")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_allocations WHERE id = ? AND month_id = ?`, allocationID, monthID).Scan(&count); err != nil {
		return fmt.Errorf("check wallet allocation: %w", err)
	}
	if count == 0 {
		return errors.New("allocation does not belong to this wallet month")
	}
	return nil
}

func (r *Repository) ensureCategoryExists(ctx context.Context, categoryID string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_categories WHERE id = ? AND active = 1`, categoryID).Scan(&count); err != nil {
		return fmt.Errorf("check wallet category: %w", err)
	}
	if count == 0 {
		return errors.New("category does not exist")
	}
	return nil
}

func (r *Repository) ensureAllocationTemplateExists(ctx context.Context, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return errors.New("template_id is required")
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_allocation_templates WHERE id = ?`, templateID).Scan(&count); err != nil {
		return fmt.Errorf("check wallet allocation template: %w", err)
	}
	if count == 0 {
		return errors.New("allocation template does not exist")
	}
	return nil
}

func (r *Repository) ensureMonthOpen(ctx context.Context, monthID string) error {
	var status string
	if err := r.db.QueryRowContext(ctx, `SELECT status FROM wallet_months WHERE id = ?`, monthID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return shared.ErrNotFound
		}
		return fmt.Errorf("check wallet month status: %w", err)
	}
	if status == "closed" {
		return errors.New("wallet month is closed")
	}
	return nil
}

func deleteByID(ctx context.Context, database *db.DB, table string, id string) error {
	result, err := database.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete %s: %w", table, err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return shared.ErrNotFound
	}
	return nil
}

func scanMonth(scanner interface {
	Scan(dest ...any) error
}) (Month, error) {
	var month Month
	var closedAt sql.NullString
	var closedWallet sql.NullInt64
	if err := scanner.Scan(&month.ID, &month.Month, &month.OpeningBalanceCents, &month.WalletBalanceCents,
		&month.Status, &closedAt, &closedWallet, &month.CreatedAt, &month.UpdatedAt); err != nil {
		return Month{}, err
	}
	month.ClosedAt = shared.FromNullString(closedAt)
	month.ClosedWalletBalanceCents = ptrInt64(closedWallet)
	return month, nil
}

func scanIncome(scanner interface {
	Scan(dest ...any) error
}) (IncomeItem, error) {
	var item IncomeItem
	var receivedDate, notes sql.NullString
	if err := scanner.Scan(&item.ID, &item.MonthID, &item.Name, &item.AmountCents, &receivedDate,
		&item.AppliesToMonth, &notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return IncomeItem{}, err
	}
	item.ReceivedDate = shared.FromNullString(receivedDate)
	item.Notes = shared.FromNullString(notes)
	return item, nil
}

func scanAllocation(scanner interface {
	Scan(dest ...any) error
}) (Allocation, error) {
	var allocation Allocation
	var templateID sql.NullString
	var carryForward, active int
	if err := scanner.Scan(&allocation.ID, &allocation.MonthID, &templateID, &allocation.Name,
		&allocation.BudgetedCents, &allocation.Type, &carryForward, &allocation.SortOrder, &active,
		&allocation.CreatedAt, &allocation.UpdatedAt); err != nil {
		return Allocation{}, err
	}
	allocation.TemplateID = shared.FromNullString(templateID)
	allocation.CarryForward = intBool(carryForward)
	allocation.Active = intBool(active)
	return allocation, nil
}

func scanCategory(scanner interface {
	Scan(dest ...any) error
}) (Category, error) {
	var category Category
	var systemKey sql.NullString
	var active int
	if err := scanner.Scan(&category.ID, &category.Name, &systemKey, &active, &category.SortOrder,
		&category.CreatedAt, &category.UpdatedAt); err != nil {
		return Category{}, err
	}
	category.SystemKey = shared.FromNullString(systemKey)
	category.Active = intBool(active)
	return category, nil
}

func scanTransaction(scanner interface {
	Scan(dest ...any) error
}) (Transaction, error) {
	var transaction Transaction
	var note, parent sql.NullString
	var rounded int
	if err := scanner.Scan(&transaction.ID, &transaction.MonthID, &transaction.AllocationID,
		&transaction.AllocationName, &transaction.CategoryID, &transaction.CategoryName, &transaction.Date,
		&transaction.AmountCents, &note, &rounded, &transaction.Kind, &transaction.Source, &parent,
		&transaction.SplitChildCount, &transaction.CreatedAt, &transaction.UpdatedAt); err != nil {
		return Transaction{}, err
	}
	transaction.Note = shared.FromNullString(note)
	transaction.Rounded = intBool(rounded)
	transaction.ParentTransactionID = shared.FromNullString(parent)
	if transaction.ParentTransactionID != nil {
		transaction.SplitRole = "child"
	} else if transaction.SplitChildCount > 0 {
		transaction.SplitRole = "parent"
	}
	return transaction, nil
}

func transactionSelectSQL() string {
	return `SELECT t.id, t.month_id, t.allocation_id, a.name AS allocation_name,
			t.category_id, c.name AS category_name, t.date, t.amount_cents, t.note, t.rounded,
			t.kind, t.source, t.parent_transaction_id,
			(SELECT COUNT(*) FROM wallet_transaction_splits split_count WHERE split_count.parent_transaction_id = t.id) AS split_child_count,
			t.created_at, t.updated_at
		FROM wallet_transactions t
		JOIN wallet_allocations a ON a.id = t.allocation_id
		JOIN wallet_categories c ON c.id = t.category_id`
}
