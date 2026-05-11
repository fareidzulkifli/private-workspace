package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"private-workspace/internal/shared"
)

func (r *Repository) SplitTransaction(ctx context.Context, id string, req CreateTransactionSplitRequest) (TransactionSplitResult, error) {
	parent, err := r.GetTransaction(ctx, id)
	if err != nil {
		return TransactionSplitResult{}, err
	}
	if err := r.ensureMonthOpen(ctx, parent.MonthID); err != nil {
		return TransactionSplitResult{}, err
	}
	if parent.ParentTransactionID != nil {
		return TransactionSplitResult{}, errors.New("split detail cannot be split again")
	}
	if parent.Kind != "spend" {
		return TransactionSplitResult{}, errors.New("only spending transactions can be split")
	}
	hasChildren, err := r.transactionHasSplitChildren(ctx, parent.ID)
	if err != nil {
		return TransactionSplitResult{}, err
	}
	if hasChildren {
		return TransactionSplitResult{}, errors.New("transaction is already split")
	}
	if len(req.Splits) < 2 {
		return TransactionSplitResult{}, errors.New("at least two split rows are required")
	}
	if len(req.Splits) > 20 {
		return TransactionSplitResult{}, errors.New("split rows cannot exceed 20")
	}

	children := make([]Transaction, 0, len(req.Splits))
	var total int64
	now := shared.Now()
	for _, input := range req.Splits {
		child, err := r.sanitizeSplitInput(ctx, parent, input, now)
		if err != nil {
			return TransactionSplitResult{}, err
		}
		total += child.AmountCents
		children = append(children, child)
	}
	if total != parent.AmountCents {
		return TransactionSplitResult{}, errors.New("split rows must sum to the original transaction amount")
	}

	err = r.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, child := range children {
			if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_transactions
				(id, month_id, allocation_id, category_id, date, amount_cents, note, rounded, kind, source,
					parent_transaction_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'spend', 'split', ?, ?, ?)`,
				child.ID, child.MonthID, child.AllocationID, child.CategoryID, child.Date, child.AmountCents,
				shared.NullString(child.Note), boolInt(child.Rounded), parent.ID, child.CreatedAt, child.UpdatedAt); err != nil {
				return fmt.Errorf("create split child transaction: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO wallet_transaction_splits
				(id, parent_transaction_id, child_transaction_id, created_at)
				VALUES (?, ?, ?, ?)`,
				shared.NewID(), parent.ID, child.ID, now); err != nil {
				return fmt.Errorf("create transaction split link: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE wallet_transactions
			SET updated_at = ?
			WHERE id = ?`, now, parent.ID); err != nil {
			return fmt.Errorf("touch split parent: %w", err)
		}
		return nil
	})
	if err != nil {
		return TransactionSplitResult{}, err
	}

	savedChildren := make([]Transaction, 0, len(children))
	for _, child := range children {
		saved, err := r.GetTransaction(ctx, child.ID)
		if err != nil {
			return TransactionSplitResult{}, err
		}
		savedChildren = append(savedChildren, saved)
	}
	parent, err = r.GetTransaction(ctx, parent.ID)
	if err != nil {
		return TransactionSplitResult{}, err
	}
	return TransactionSplitResult{Parent: parent, Children: savedChildren}, nil
}

func (r *Repository) SplitTransactionDetail(ctx context.Context, id string) (TransactionSplitDetail, error) {
	transaction, err := r.GetTransaction(ctx, id)
	if err != nil {
		return TransactionSplitDetail{}, err
	}
	parentID := transaction.ID
	if transaction.ParentTransactionID != nil {
		parentID = *transaction.ParentTransactionID
	}
	parent, err := r.GetTransaction(ctx, parentID)
	if err != nil {
		return TransactionSplitDetail{}, err
	}
	children, err := r.splitChildren(ctx, parent.ID)
	if err != nil {
		return TransactionSplitDetail{}, err
	}
	if len(children) == 0 {
		return TransactionSplitDetail{}, errors.New("transaction is not split")
	}
	return TransactionSplitDetail{Parent: parent, Children: children}, nil
}

func (r *Repository) sanitizeSplitInput(ctx context.Context, parent Transaction, input TransactionSplitInput, now string) (Transaction, error) {
	allocationID := strings.TrimSpace(input.AllocationID)
	if allocationID == "" {
		allocationID = parent.AllocationID
	}
	if err := r.ensureAllocationBelongsToMonth(ctx, allocationID, parent.MonthID); err != nil {
		return Transaction{}, err
	}
	categoryID := strings.TrimSpace(input.CategoryID)
	if categoryID == "" {
		categoryID = parent.CategoryID
	}
	if err := r.ensureCategoryExists(ctx, categoryID); err != nil {
		return Transaction{}, err
	}
	if input.AmountCents <= 0 {
		return Transaction{}, errors.New("split amount must be greater than zero")
	}
	date := strings.TrimSpace(input.Date)
	if date == "" {
		date = parent.Date
	}
	var err error
	date, err = validateDate(date, "date")
	if err != nil {
		return Transaction{}, err
	}
	return Transaction{
		ID:                  shared.NewID(),
		MonthID:             parent.MonthID,
		AllocationID:        allocationID,
		CategoryID:          categoryID,
		Date:                date,
		AmountCents:         input.AmountCents,
		Note:                optionalStringPtr(input.Note),
		Rounded:             input.Rounded,
		Kind:                "spend",
		Source:              "split",
		ParentTransactionID: &parent.ID,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (r *Repository) transactionHasSplitChildren(ctx context.Context, id string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_transaction_splits WHERE parent_transaction_id = ?`, id).Scan(&count); err != nil {
		return false, fmt.Errorf("check split children: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) splitChildren(ctx context.Context, parentID string) ([]Transaction, error) {
	rows, err := r.db.QueryContext(ctx, transactionSelectSQL()+`
		WHERE t.parent_transaction_id = ?
		ORDER BY t.date ASC, t.created_at ASC`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list split children: %w", err)
	}
	defer rows.Close()
	children := []Transaction{}
	for rows.Next() {
		child, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return children, rows.Err()
}
