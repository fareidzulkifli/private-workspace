package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"private-workspace/internal/shared"
)

func (r *Repository) MonthlyReport(ctx context.Context, from string, to string) ([]MonthlyReportRow, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		months, err := r.ListMonths(ctx)
		if err != nil {
			return nil, err
		}
		if len(months) == 0 {
			return []MonthlyReportRow{}, nil
		}
		if to == "" {
			to = months[0].Month
		}
		if from == "" {
			index := 5
			if len(months)-1 < index {
				index = len(months) - 1
			}
			from = months[index].Month
		}
	}
	var err error
	from, err = validateMonth(from)
	if err != nil {
		return nil, err
	}
	to, err = validateMonth(to)
	if err != nil {
		return nil, err
	}
	if from > to {
		return nil, errors.New("from month must be before to month")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, month, opening_balance_cents, wallet_balance_cents,
			status, closed_at, closed_wallet_balance_cents, created_at, updated_at
		FROM wallet_months
		WHERE month >= ? AND month <= ?
		ORDER BY month ASC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("wallet monthly report: %w", err)
	}

	var reportMonths []Month
	for rows.Next() {
		month, err := scanMonth(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		reportMonths = append(reportMonths, month)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	var report []MonthlyReportRow
	for _, month := range reportMonths {
		_, incomeTotal, err := r.listIncomeForMonth(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		_, totalReserved, spendingTotal, err := r.listAllocationSummaries(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		adjustmentTotal, err := r.adjustmentTotal(ctx, month.ID)
		if err != nil {
			return nil, err
		}
		expected := month.OpeningBalanceCents + incomeTotal - spendingTotal + adjustmentTotal
		report = append(report, MonthlyReportRow{
			Month:                 month.Month,
			Status:                month.Status,
			OpeningBalanceCents:   month.OpeningBalanceCents,
			IncomeTotalCents:      incomeTotal,
			SpendingTotalCents:    spendingTotal,
			AdjustmentTotalCents:  adjustmentTotal,
			ExpectedBalanceCents:  expected,
			WalletBalanceCents:    month.WalletBalanceCents,
			VarianceCents:         month.WalletBalanceCents - expected,
			TotalReservedCents:    totalReserved,
			AvailableBalanceCents: month.WalletBalanceCents - totalReserved,
		})
	}
	if report == nil {
		report = []MonthlyReportRow{}
	}
	return report, nil
}

func (r *Repository) AllocationReport(ctx context.Context, monthKey string) ([]AllocationSummary, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	allocations, _, _, err := r.listAllocationSummaries(ctx, month.ID)
	return allocations, err
}

func (r *Repository) CategoryReport(ctx context.Context, monthKey string, allocationID string) ([]CategoryReportRow, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return nil, err
	}
	breakdown, err := r.categoryBreakdown(ctx, month.ID, allocationID)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, row := range breakdown {
		total += row.AmountCents
	}
	report := make([]CategoryReportRow, 0, len(breakdown))
	for _, row := range breakdown {
		percent := 0.0
		if total > 0 {
			percent = (float64(row.AmountCents) / float64(total)) * 100
		}
		report = append(report, CategoryReportRow{
			CategoryID:     row.CategoryID,
			CategoryName:   row.CategoryName,
			AmountCents:    row.AmountCents,
			Count:          row.Count,
			PercentOfSpend: percent,
		})
	}
	return report, nil
}

func (r *Repository) ReviewReport(ctx context.Context, monthKey string) (ReviewReport, error) {
	summary, err := r.Summary(ctx, monthKey)
	if err != nil {
		return ReviewReport{}, err
	}
	return ReviewReport{
		Month:                summary.Month.Month,
		ReviewCounts:         summary.ReviewCounts,
		VarianceCents:        summary.VarianceCents,
		AdjustmentTotalCents: summary.AdjustmentTotalCents,
	}, nil
}

func (r *Repository) CloseMonth(ctx context.Context, monthKey string) (Month, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return Month{}, err
	}
	if month.Status == "closed" {
		return month, nil
	}
	now := shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_months
		SET status = 'closed', closed_at = ?, closed_wallet_balance_cents = ?, updated_at = ?
		WHERE id = ?`, now, month.WalletBalanceCents, now, month.ID)
	if err != nil {
		return Month{}, fmt.Errorf("close wallet month: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Month{}, shared.ErrNotFound
	}
	return r.GetMonth(ctx, month.Month)
}

func (r *Repository) ReopenMonth(ctx context.Context, monthKey string) (Month, error) {
	month, err := r.GetMonth(ctx, monthKey)
	if err != nil {
		return Month{}, err
	}
	if month.Status == "open" {
		return month, nil
	}
	now := shared.Now()
	result, err := r.db.ExecContext(ctx, `UPDATE wallet_months
		SET status = 'open', closed_at = NULL, closed_wallet_balance_cents = NULL, updated_at = ?
		WHERE id = ?`, now, month.ID)
	if err != nil {
		return Month{}, fmt.Errorf("reopen wallet month: %w", err)
	}
	if ok, err := shared.QuerySingleResult(result); err == nil && !ok {
		return Month{}, shared.ErrNotFound
	}
	return r.GetMonth(ctx, month.Month)
}
