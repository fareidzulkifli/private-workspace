package wallet

import (
	"context"
	"fmt"
	"strings"
)

type ReviewFilters struct {
	CleanupOnly  bool
	UnsortedOnly bool
	RoundedOnly  bool
	MissingNote  bool
	AllocationID string
	CategoryID   string
	Limit        int
}

func (r *Repository) ReviewTransactions(ctx context.Context, monthKey string, filters ReviewFilters) (ReviewTransactionsResult, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return ReviewTransactionsResult{}, err
	}
	if filters.Limit <= 0 || filters.Limit > 500 {
		filters.Limit = 200
	}
	args := []any{month.ID}
	conditions := []string{"t.month_id = ?", visibleSpendCondition("t")}
	if filters.CleanupOnly {
		conditions = append(conditions, "(t.category_id = ? OR t.rounded = 1 OR t.note IS NULL OR trim(t.note) = '')")
		args = append(args, UnsortedCategoryID)
	}
	if filters.UnsortedOnly {
		conditions = append(conditions, "t.category_id = ?")
		args = append(args, UnsortedCategoryID)
	}
	if filters.RoundedOnly {
		conditions = append(conditions, "t.rounded = 1")
	}
	if filters.MissingNote {
		conditions = append(conditions, "(t.note IS NULL OR trim(t.note) = '')")
	}
	if strings.TrimSpace(filters.AllocationID) != "" {
		conditions = append(conditions, "t.allocation_id = ?")
		args = append(args, strings.TrimSpace(filters.AllocationID))
	}
	if strings.TrimSpace(filters.CategoryID) != "" {
		conditions = append(conditions, "t.category_id = ?")
		args = append(args, strings.TrimSpace(filters.CategoryID))
	}
	args = append(args, filters.Limit)

	rows, err := r.db.QueryContext(ctx, transactionSelectSQL()+`
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY t.date DESC, t.created_at DESC
		LIMIT ?`, args...)
	if err != nil {
		return ReviewTransactionsResult{}, fmt.Errorf("list wallet review transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return ReviewTransactionsResult{}, err
		}
		transactions = append(transactions, transaction)
	}
	if transactions == nil {
		transactions = []Transaction{}
	}
	return ReviewTransactionsResult{Transactions: transactions}, rows.Err()
}

func (r *Repository) AllocationDetail(ctx context.Context, allocationID string) (AllocationDetail, error) {
	allocation, err := r.GetAllocation(ctx, allocationID)
	if err != nil {
		return AllocationDetail{}, err
	}
	summary, err := r.allocationSummary(ctx, allocation.ID)
	if err != nil {
		return AllocationDetail{}, err
	}
	breakdown, err := r.categoryBreakdown(ctx, allocation.MonthID, allocation.ID)
	if err != nil {
		return AllocationDetail{}, err
	}
	transactions, err := r.transactionsForAllocation(ctx, allocation.ID, 100)
	if err != nil {
		return AllocationDetail{}, err
	}
	return AllocationDetail{
		Allocation:        summary,
		CategoryBreakdown: breakdown,
		Transactions:      transactions,
	}, nil
}

func (r *Repository) allocationSummary(ctx context.Context, allocationID string) (AllocationSummary, error) {
	row := r.db.QueryRowContext(ctx, `SELECT
			a.id, a.month_id, a.template_id, a.name, a.budgeted_cents, a.type,
			a.carry_forward, a.sort_order, a.active, a.created_at, a.updated_at,
			COALESCE(SUM(t.amount_cents), 0) AS spent_cents
		FROM wallet_allocations a
		LEFT JOIN wallet_transactions t ON t.allocation_id = a.id AND `+visibleSpendCondition("t")+`
		WHERE a.id = ?
		GROUP BY a.id`, allocationID)
	var allocation Allocation
	var spent int64
	scanner := allocationSpendScanner{rows: row, allocation: &allocation, spent: &spent}
	if err := scanner.scan(); err != nil {
		return AllocationSummary{}, err
	}
	summary := AllocationSummary{
		Allocation:     allocation,
		SpentCents:     spent,
		RemainingCents: allocation.BudgetedCents - spent,
	}
	summaries, err := r.attachDefaultCategoriesToSummaries(ctx, []AllocationSummary{summary})
	if err != nil {
		return AllocationSummary{}, err
	}
	return summaries[0], nil
}

func (r *Repository) categoryBreakdown(ctx context.Context, monthID string, allocationID string) ([]CategoryBreakdownRow, error) {
	args := []any{monthID}
	filter := ""
	if strings.TrimSpace(allocationID) != "" {
		filter = " AND t.allocation_id = ?"
		args = append(args, strings.TrimSpace(allocationID))
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.name, COALESCE(SUM(t.amount_cents), 0), COUNT(t.id)
		FROM wallet_transactions t
		JOIN wallet_categories c ON c.id = t.category_id
		WHERE t.month_id = ? AND `+visibleSpendCondition("t")+filter+`
		GROUP BY c.id, c.name
		ORDER BY COALESCE(SUM(t.amount_cents), 0) DESC, c.name ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("wallet category breakdown: %w", err)
	}
	defer rows.Close()

	var breakdown []CategoryBreakdownRow
	for rows.Next() {
		var row CategoryBreakdownRow
		if err := rows.Scan(&row.CategoryID, &row.CategoryName, &row.AmountCents, &row.Count); err != nil {
			return nil, err
		}
		breakdown = append(breakdown, row)
	}
	if breakdown == nil {
		breakdown = []CategoryBreakdownRow{}
	}
	return breakdown, rows.Err()
}

func (r *Repository) transactionsForAllocation(ctx context.Context, allocationID string, limit int) ([]Transaction, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, transactionSelectSQL()+`
		WHERE t.allocation_id = ? AND `+visibleSpendCondition("t")+`
		ORDER BY t.date DESC, t.created_at DESC
		LIMIT ?`, allocationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list wallet allocation transactions: %w", err)
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
