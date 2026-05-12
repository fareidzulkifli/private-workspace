package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"private-workspace/internal/shared"
)

func (r *Repository) CreateBalanceUpdate(ctx context.Context, monthKey string, req CreateBalanceUpdateRequest) (BalanceUpdateResult, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return BalanceUpdateResult{}, err
	}
	if month.Status == "closed" {
		return BalanceUpdateResult{}, errors.New("wallet month is closed")
	}
	expectedBalance, err := r.expectedBalance(ctx, month.ID)
	if err != nil {
		return BalanceUpdateResult{}, err
	}

	now := shared.Now()
	update := BalanceUpdate{
		ID:                   shared.NewID(),
		MonthID:              month.ID,
		PreviousBalanceCents: month.WalletBalanceCents,
		NewBalanceCents:      req.NewBalanceCents,
		ExpectedBalanceCents: expectedBalance,
		DifferenceCents:      req.NewBalanceCents - expectedBalance,
		Note:                 optionalStringPtr(req.Note),
		CreatedAt:            now,
	}
	balanceDelta := req.NewBalanceCents - month.WalletBalanceCents
	var transactionID string

	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := insertBalanceUpdate(ctx, tx, update); err != nil {
			return err
		}
		if balanceDelta != 0 {
			transaction, err := createReconciliationTransaction(ctx, tx, month, balanceDelta, req.Note, now)
			if err != nil {
				return err
			}
			transactionID = transaction.ID
		}
		if _, err := tx.ExecContext(ctx, `UPDATE wallet_months
			SET wallet_balance_cents = ?, updated_at = ?
			WHERE id = ?`, update.NewBalanceCents, now, month.ID); err != nil {
			return fmt.Errorf("update wallet balance: %w", err)
		}
		return nil
	})
	if err != nil {
		return BalanceUpdateResult{}, err
	}
	result := BalanceUpdateResult{BalanceUpdate: update}
	if transactionID != "" {
		transaction, err := r.GetTransaction(ctx, transactionID)
		if err != nil {
			return BalanceUpdateResult{}, err
		}
		result.Transaction = &transaction
	}
	return result, nil
}

func createReconciliationTransaction(ctx context.Context, q shared.SQLer, month Month, balanceDelta int64, note *string, now string) (Transaction, error) {
	allocationID, err := ensureReconciliationAllocation(ctx, q, month.ID, now)
	if err != nil {
		return Transaction{}, err
	}
	categoryID, err := ensureReconciliationCategory(ctx, q, now)
	if err != nil {
		return Transaction{}, err
	}

	kind := "income"
	amount := balanceDelta
	if balanceDelta < 0 {
		kind = "spend"
		amount = -balanceDelta
	}
	transaction := Transaction{
		ID:             shared.NewID(),
		MonthID:        month.ID,
		AllocationID:   allocationID,
		CategoryID:     categoryID,
		Date:           reconciliationTransactionDate(month.Month),
		AmountCents:    amount,
		Note:           reconciliationTransactionNote(note),
		Rounded:        false,
		Kind:           kind,
		Source:         "reconciliation",
		CreatedAt:      now,
		UpdatedAt:      now,
		AllocationName: ReconciliationAllocationName,
		CategoryName:   ReconciliationCategoryName,
	}
	if err := insertReconciliationTransaction(ctx, q, transaction); err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}

func ensureReconciliationAllocation(ctx context.Context, q shared.SQLer, monthID string, now string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id
		FROM wallet_allocations
		WHERE month_id = ? AND name = ?
		ORDER BY active ASC, created_at ASC
		LIMIT 1`, monthID, ReconciliationAllocationName).Scan(&id)
	if err == nil {
		if _, err := q.ExecContext(ctx, `UPDATE wallet_allocations
			SET budgeted_cents = 0, type = 'flexible', carry_forward = 0, sort_order = ?, active = 0, updated_at = ?
			WHERE id = ?`, reconciliationAllocationSortOrder, now, id); err != nil {
			return "", fmt.Errorf("update wallet reconciliation allocation: %w", err)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find wallet reconciliation allocation: %w", err)
	}

	id = shared.NewID()
	if _, err := q.ExecContext(ctx, `INSERT INTO wallet_allocations
		(id, month_id, name, budgeted_cents, type, carry_forward, sort_order, active, created_at, updated_at)
		VALUES (?, ?, ?, 0, 'flexible', 0, ?, 0, ?, ?)`,
		id, monthID, ReconciliationAllocationName, reconciliationAllocationSortOrder, now, now); err != nil {
		return "", fmt.Errorf("create wallet reconciliation allocation: %w", err)
	}
	return id, nil
}

func ensureReconciliationCategory(ctx context.Context, q shared.SQLer, now string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, `SELECT id
		FROM wallet_categories
		WHERE id = ? OR system_key = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END
		LIMIT 1`, ReconciliationCategoryID, ReconciliationCategorySystemKey, ReconciliationCategoryID).Scan(&id)
	if err == nil {
		if _, err := q.ExecContext(ctx, `UPDATE wallet_categories
			SET name = ?, system_key = ?, active = 1, sort_order = ?, updated_at = ?
			WHERE id = ?`, ReconciliationCategoryName, ReconciliationCategorySystemKey, reconciliationAllocationSortOrder, now, id); err != nil {
			return "", fmt.Errorf("update wallet reconciliation category: %w", err)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find wallet reconciliation category: %w", err)
	}

	err = q.QueryRowContext(ctx, `SELECT id
		FROM wallet_categories
		WHERE name = ?
		LIMIT 1`, ReconciliationCategoryName).Scan(&id)
	if err == nil {
		if _, err := q.ExecContext(ctx, `UPDATE wallet_categories
			SET system_key = ?, active = 1, sort_order = ?, updated_at = ?
			WHERE id = ?`, ReconciliationCategorySystemKey, reconciliationAllocationSortOrder, now, id); err != nil {
			return "", fmt.Errorf("claim wallet reconciliation category: %w", err)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find wallet adjustment category by name: %w", err)
	}

	if _, err := q.ExecContext(ctx, `INSERT INTO wallet_categories
		(id, name, system_key, active, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?, ?)`,
		ReconciliationCategoryID, ReconciliationCategoryName, ReconciliationCategorySystemKey, reconciliationAllocationSortOrder, now, now); err != nil {
		return "", fmt.Errorf("create wallet reconciliation category: %w", err)
	}
	return ReconciliationCategoryID, nil
}

func insertReconciliationTransaction(ctx context.Context, q shared.SQLer, transaction Transaction) error {
	_, err := q.ExecContext(ctx, `INSERT INTO wallet_transactions
		(id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'reconciliation', ?, ?)`,
		transaction.ID, transaction.MonthID, transaction.AllocationID, transaction.CategoryID, transaction.Date,
		transaction.AmountCents, shared.NullString(transaction.Note), boolInt(transaction.Rounded), transaction.Kind,
		transaction.CreatedAt, transaction.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create wallet reconciliation transaction: %w", err)
	}
	return nil
}

func reconciliationTransactionNote(note *string) *string {
	if trimmed := optionalStringPtr(note); trimmed != nil {
		return trimmed
	}
	value := "Reconcile balance"
	return &value
}

func reconciliationTransactionDate(monthKey string) string {
	today := time.Now().Format("2006-01-02")
	if strings.HasPrefix(today, monthKey+"-") {
		return today
	}
	return monthKey + "-01"
}

func (r *Repository) ListBalanceUpdates(ctx context.Context, monthKey string, limit int) ([]BalanceUpdate, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, month_id, previous_balance_cents, new_balance_cents,
			expected_balance_cents, difference_cents, note, created_at
		FROM wallet_balance_updates
		WHERE month_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, month.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list wallet balance updates: %w", err)
	}
	defer rows.Close()

	var updates []BalanceUpdate
	for rows.Next() {
		update, err := scanBalanceUpdate(rows)
		if err != nil {
			return nil, err
		}
		updates = append(updates, update)
	}
	if updates == nil {
		updates = []BalanceUpdate{}
	}
	return updates, rows.Err()
}

func (r *Repository) CreateReconciliationAdjustment(ctx context.Context, monthKey string, req CreateReconciliationAdjustmentRequest) (ReconciliationAdjustment, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return ReconciliationAdjustment{}, err
	}
	if month.Status == "closed" {
		return ReconciliationAdjustment{}, errors.New("wallet month is closed")
	}
	if req.AmountCents == 0 {
		return ReconciliationAdjustment{}, errors.New("adjustment amount cannot be zero")
	}
	reason, err := normalizeAdjustmentReason(req.Reason)
	if err != nil {
		return ReconciliationAdjustment{}, err
	}
	if req.BalanceUpdateID != nil {
		if err := r.ensureBalanceUpdateBelongsToMonth(ctx, *req.BalanceUpdateID, month.ID); err != nil {
			return ReconciliationAdjustment{}, err
		}
	}
	adjustment := ReconciliationAdjustment{
		ID:              shared.NewID(),
		MonthID:         month.ID,
		BalanceUpdateID: optionalStringPtr(req.BalanceUpdateID),
		AmountCents:     req.AmountCents,
		Reason:          reason,
		Note:            optionalStringPtr(req.Note),
		CreatedAt:       shared.Now(),
	}
	if err := insertReconciliationAdjustment(ctx, r.db, adjustment); err != nil {
		return ReconciliationAdjustment{}, err
	}
	return adjustment, nil
}

func (r *Repository) ListReconciliationAdjustments(ctx context.Context, monthKey string, limit int) ([]ReconciliationAdjustment, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, month_id, balance_update_id, amount_cents,
			reason, note, created_at
		FROM wallet_reconciliation_adjustments
		WHERE month_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, month.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("list wallet reconciliation adjustments: %w", err)
	}
	defer rows.Close()

	var adjustments []ReconciliationAdjustment
	for rows.Next() {
		adjustment, err := scanReconciliationAdjustment(rows)
		if err != nil {
			return nil, err
		}
		adjustments = append(adjustments, adjustment)
	}
	if adjustments == nil {
		adjustments = []ReconciliationAdjustment{}
	}
	return adjustments, rows.Err()
}

func (r *Repository) expectedBalance(ctx context.Context, monthID string) (int64, error) {
	var opening int64
	if err := r.db.QueryRowContext(ctx, `SELECT opening_balance_cents FROM wallet_months WHERE id = ?`, monthID).Scan(&opening); err != nil {
		return 0, fmt.Errorf("get wallet opening balance: %w", err)
	}
	var incomeTotal int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents), 0)
		FROM wallet_income_items
		WHERE month_id = ?`, monthID).Scan(&incomeTotal); err != nil {
		return 0, fmt.Errorf("sum wallet income: %w", err)
	}
	incomeTransactionTotal, err := r.incomeTransactionTotal(ctx, monthID)
	if err != nil {
		return 0, err
	}
	incomeTotal += incomeTransactionTotal
	var spendingTotal int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(t.amount_cents), 0)
		FROM wallet_transactions t
		WHERE t.month_id = ? AND `+visibleSpendCondition("t"), monthID).Scan(&spendingTotal); err != nil {
		return 0, fmt.Errorf("sum wallet spending: %w", err)
	}
	return opening + incomeTotal - spendingTotal, nil
}

func (r *Repository) ensureBalanceUpdateBelongsToMonth(ctx context.Context, balanceUpdateID string, monthID string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_balance_updates WHERE id = ? AND month_id = ?`,
		balanceUpdateID, monthID).Scan(&count); err != nil {
		return fmt.Errorf("check wallet balance update: %w", err)
	}
	if count == 0 {
		return errors.New("balance update does not belong to this wallet month")
	}
	return nil
}

func insertBalanceUpdate(ctx context.Context, q shared.SQLer, update BalanceUpdate) error {
	_, err := q.ExecContext(ctx, `INSERT INTO wallet_balance_updates
		(id, month_id, previous_balance_cents, new_balance_cents, expected_balance_cents,
			difference_cents, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		update.ID, update.MonthID, update.PreviousBalanceCents, update.NewBalanceCents,
		update.ExpectedBalanceCents, update.DifferenceCents, shared.NullString(update.Note), update.CreatedAt)
	if err != nil {
		return fmt.Errorf("create wallet balance update: %w", err)
	}
	return nil
}

func insertReconciliationAdjustment(ctx context.Context, q shared.SQLer, adjustment ReconciliationAdjustment) error {
	_, err := q.ExecContext(ctx, `INSERT INTO wallet_reconciliation_adjustments
		(id, month_id, balance_update_id, amount_cents, reason, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		adjustment.ID, adjustment.MonthID, shared.NullString(adjustment.BalanceUpdateID),
		adjustment.AmountCents, adjustment.Reason, shared.NullString(adjustment.Note), adjustment.CreatedAt)
	if err != nil {
		return fmt.Errorf("create wallet reconciliation adjustment: %w", err)
	}
	return nil
}

func scanBalanceUpdate(scanner interface {
	Scan(dest ...any) error
}) (BalanceUpdate, error) {
	var update BalanceUpdate
	var note sql.NullString
	if err := scanner.Scan(&update.ID, &update.MonthID, &update.PreviousBalanceCents,
		&update.NewBalanceCents, &update.ExpectedBalanceCents, &update.DifferenceCents,
		&note, &update.CreatedAt); err != nil {
		return BalanceUpdate{}, err
	}
	update.Note = shared.FromNullString(note)
	return update, nil
}

func scanReconciliationAdjustment(scanner interface {
	Scan(dest ...any) error
}) (ReconciliationAdjustment, error) {
	var adjustment ReconciliationAdjustment
	var balanceUpdateID, note sql.NullString
	if err := scanner.Scan(&adjustment.ID, &adjustment.MonthID, &balanceUpdateID,
		&adjustment.AmountCents, &adjustment.Reason, &note, &adjustment.CreatedAt); err != nil {
		return ReconciliationAdjustment{}, err
	}
	adjustment.BalanceUpdateID = shared.FromNullString(balanceUpdateID)
	adjustment.Note = shared.FromNullString(note)
	return adjustment, nil
}
