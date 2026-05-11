package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
	var adjustment *ReconciliationAdjustment
	if req.CreateAdjustment && update.DifferenceCents != 0 {
		reason, err := normalizeAdjustmentReason(req.AdjustmentReason)
		if err != nil {
			return BalanceUpdateResult{}, err
		}
		adjustment = &ReconciliationAdjustment{
			ID:              shared.NewID(),
			MonthID:         month.ID,
			BalanceUpdateID: &update.ID,
			AmountCents:     update.DifferenceCents,
			Reason:          reason,
			Note:            optionalStringPtr(req.AdjustmentNote),
			CreatedAt:       now,
		}
	}

	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := insertBalanceUpdate(ctx, tx, update); err != nil {
			return err
		}
		if adjustment != nil {
			if err := insertReconciliationAdjustment(ctx, tx, *adjustment); err != nil {
				return err
			}
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
	return BalanceUpdateResult{BalanceUpdate: update, Adjustment: adjustment}, nil
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
	var spendingTotal int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(t.amount_cents), 0)
		FROM wallet_transactions t
		WHERE t.month_id = ? AND `+visibleSpendCondition("t"), monthID).Scan(&spendingTotal); err != nil {
		return 0, fmt.Errorf("sum wallet spending: %w", err)
	}
	adjustmentTotal, err := r.adjustmentTotal(ctx, monthID)
	if err != nil {
		return 0, err
	}
	return opening + incomeTotal - spendingTotal + adjustmentTotal, nil
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
