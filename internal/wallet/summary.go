package wallet

import (
	"context"
	"fmt"

	"private-workspace/internal/shared"
)

func (r *Repository) Summary(ctx context.Context, monthKey string) (MonthSummary, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return MonthSummary{}, err
	}
	incomeItems, incomeTotal, err := r.listIncomeForMonth(ctx, month.ID)
	if err != nil {
		return MonthSummary{}, err
	}
	incomeTransactionTotal, err := r.incomeTransactionTotal(ctx, month.ID)
	if err != nil {
		return MonthSummary{}, err
	}
	incomeTotal += incomeTransactionTotal
	allocations, totalReserved, spendingTotal, err := r.listAllocationSummaries(ctx, month.ID)
	if err != nil {
		return MonthSummary{}, err
	}
	adjustmentTotal, err := r.adjustmentTotal(ctx, month.ID)
	if err != nil {
		return MonthSummary{}, err
	}
	balanceUpdates, err := r.ListBalanceUpdates(ctx, month.Month, 5)
	if err != nil {
		return MonthSummary{}, err
	}
	adjustments, err := r.ListReconciliationAdjustments(ctx, month.Month, 5)
	if err != nil {
		return MonthSummary{}, err
	}
	recent, err := r.recentTransactions(ctx, month.ID, 20)
	if err != nil {
		return MonthSummary{}, err
	}
	categories, err := r.ListCategories(ctx)
	if err != nil {
		return MonthSummary{}, err
	}
	reviewCounts, err := r.reviewCounts(ctx, month.ID)
	if err != nil {
		return MonthSummary{}, err
	}
	expectedBalance := month.OpeningBalanceCents + incomeTotal - spendingTotal
	return MonthSummary{
		Month:                 month,
		IncomeItems:           incomeItems,
		Allocations:           allocations,
		RecentTransactions:    recent,
		Categories:            categories,
		BalanceUpdates:        balanceUpdates,
		Adjustments:           adjustments,
		ReviewCounts:          reviewCounts,
		IncomeTotalCents:      incomeTotal,
		SpendingTotalCents:    spendingTotal,
		AdjustmentTotalCents:  adjustmentTotal,
		ExpectedBalanceCents:  expectedBalance,
		WalletBalanceCents:    month.WalletBalanceCents,
		VarianceCents:         month.WalletBalanceCents - expectedBalance,
		TotalReservedCents:    totalReserved,
		AvailableBalanceCents: month.WalletBalanceCents - totalReserved,
	}, nil
}

func (r *Repository) listIncomeForMonth(ctx context.Context, monthID string) ([]IncomeItem, int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, month_id, name, amount_cents, received_date,
			applies_to_month, notes, created_at, updated_at
		FROM wallet_income_items
		WHERE month_id = ?
		ORDER BY received_date IS NULL, received_date ASC, created_at ASC`, monthID)
	if err != nil {
		return nil, 0, fmt.Errorf("list wallet income: %w", err)
	}
	defer rows.Close()

	var total int64
	var items []IncomeItem
	for rows.Next() {
		item, err := scanIncome(rows)
		if err != nil {
			return nil, 0, err
		}
		total += item.AmountCents
		items = append(items, item)
	}
	if items == nil {
		items = []IncomeItem{}
	}
	return items, total, rows.Err()
}

func (r *Repository) listAllocationSummaries(ctx context.Context, monthID string) ([]AllocationSummary, int64, int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
			a.id, a.month_id, a.template_id, a.name, a.budgeted_cents, a.type,
			a.carry_forward, a.sort_order, a.active, a.created_at, a.updated_at,
			COALESCE(SUM(t.amount_cents), 0) AS spent_cents
		FROM wallet_allocations a
		LEFT JOIN wallet_transactions t ON t.allocation_id = a.id AND `+visibleSpendCondition("t")+`
		WHERE a.month_id = ?
		GROUP BY a.id
		ORDER BY a.active DESC, a.sort_order ASC, a.created_at ASC`, monthID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list wallet allocations: %w", err)
	}
	defer rows.Close()

	var totalReserved int64
	var spendingTotal int64
	var allocations []AllocationSummary
	for rows.Next() {
		var allocation Allocation
		var spent int64
		var scanner = allocationSpendScanner{rows: rows, allocation: &allocation, spent: &spent}
		if err := scanner.scan(); err != nil {
			return nil, 0, 0, err
		}
		spendingTotal += spent
		if isReconciliationAllocationName(allocation.Name) {
			continue
		}
		remaining := allocation.BudgetedCents - spent
		if allocation.Active && remaining > 0 {
			totalReserved += remaining
		}
		allocations = append(allocations, AllocationSummary{
			Allocation:     allocation,
			SpentCents:     spent,
			RemainingCents: remaining,
		})
	}
	if allocations == nil {
		allocations = []AllocationSummary{}
	}
	allocations, err = r.attachDefaultCategoriesToSummaries(ctx, allocations)
	if err != nil {
		return nil, 0, 0, err
	}
	return allocations, totalReserved, spendingTotal, rows.Err()
}

func (r *Repository) incomeTransactionTotal(ctx context.Context, monthID string) (int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(t.amount_cents), 0)
		FROM wallet_transactions t
		WHERE t.month_id = ? AND `+visibleIncomeCondition("t"), monthID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum wallet income transactions: %w", err)
	}
	return total, nil
}

type allocationSpendScanner struct {
	rows       interface{ Scan(dest ...any) error }
	allocation *Allocation
	spent      *int64
}

func (s allocationSpendScanner) scan() error {
	var templateID = shared.NullString(nil)
	var carryForward, active int
	if err := s.rows.Scan(&s.allocation.ID, &s.allocation.MonthID, &templateID, &s.allocation.Name,
		&s.allocation.BudgetedCents, &s.allocation.Type, &carryForward, &s.allocation.SortOrder,
		&active, &s.allocation.CreatedAt, &s.allocation.UpdatedAt, s.spent); err != nil {
		return fmt.Errorf("scan wallet allocation summary: %w", err)
	}
	s.allocation.TemplateID = shared.FromNullString(templateID)
	s.allocation.CarryForward = intBool(carryForward)
	s.allocation.Active = intBool(active)
	return nil
}

func (r *Repository) adjustmentTotal(ctx context.Context, monthID string) (int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents), 0)
		FROM wallet_reconciliation_adjustments
		WHERE month_id = ?`, monthID).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum wallet adjustments: %w", err)
	}
	return total, nil
}

func (r *Repository) recentTransactions(ctx context.Context, monthID string, limit int) ([]Transaction, error) {
	rows, err := r.db.QueryContext(ctx, transactionSelectSQL()+`
		WHERE t.month_id = ? AND t.kind IN ('spend', 'income')
		ORDER BY t.date DESC, t.created_at DESC
		LIMIT ?`, monthID, limit)
	if err != nil {
		return nil, fmt.Errorf("list wallet transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	if transactions == nil {
		transactions = []Transaction{}
	}
	return transactions, rows.Err()
}

func (r *Repository) reviewCounts(ctx context.Context, monthID string) (ReviewCounts, error) {
	var counts ReviewCounts
	if err := r.db.QueryRowContext(ctx, `SELECT
			COUNT(CASE WHEN category_id = ? THEN 1 END),
			COALESCE(SUM(CASE WHEN category_id = ? THEN amount_cents ELSE 0 END), 0),
			COUNT(CASE WHEN rounded = 1 THEN 1 END),
			COALESCE(SUM(CASE WHEN rounded = 1 THEN amount_cents ELSE 0 END), 0)
		FROM wallet_transactions t
		WHERE month_id = ? AND `+visibleSpendCondition("t"),
		UnsortedCategoryID, UnsortedCategoryID, monthID).
		Scan(&counts.UnsortedCount, &counts.UnsortedCents, &counts.RoundedCount, &counts.RoundedCents); err != nil {
		return ReviewCounts{}, fmt.Errorf("count wallet review items: %w", err)
	}
	return counts, nil
}

func (r *Repository) visibleTransactionCount(ctx context.Context, monthID string) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM wallet_transactions t
		WHERE t.month_id = ? AND `+visibleLedgerTransactionCondition("t"), monthID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count wallet visible transactions: %w", err)
	}
	return count, nil
}

func visibleSpendCondition(alias string) string {
	return alias + `.kind = 'spend' AND ` + visibleTransactionCondition(alias)
}

func visibleIncomeCondition(alias string) string {
	return alias + `.kind = 'income' AND ` + visibleTransactionCondition(alias)
}

func visibleLedgerTransactionCondition(alias string) string {
	return `((` + visibleSpendCondition(alias) + `) OR (` + visibleIncomeCondition(alias) + `))`
}

func visibleTransactionCondition(alias string) string {
	return `NOT EXISTS (
		SELECT 1 FROM wallet_transaction_splits visible_split
		WHERE visible_split.parent_transaction_id = ` + alias + `.id
	)`
}

func isReconciliationAllocationName(name string) bool {
	return name == ReconciliationAllocationName
}
