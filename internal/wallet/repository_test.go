package wallet

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"private-workspace/internal/db"
)

func TestWalletSummaryAndTransactionReviewFlow(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 100000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateIncome(ctx, "2026-05", CreateIncomeRequest{
		Name:           "Salary",
		AmountCents:    500000,
		ReceivedDate:   stringPtr("2026-04-30"),
		AppliesToMonth: "2026-05",
	}); err != nil {
		t.Fatal(err)
	}
	work, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{
		Name:          "Work Expense",
		BudgetedCents: 60000,
		Type:          "flexible",
		SortOrder:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{
		Name:          "Food",
		BudgetedCents: 30000,
		Type:          "flexible",
		SortOrder:     2,
	}); err != nil {
		t.Fatal(err)
	}
	transaction, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: work.ID,
		Date:         "2026-05-10",
		AmountCents:  2000,
		Note:         stringPtr("office day"),
		Rounded:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	summary, err := repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if summary.IncomeTotalCents != 500000 {
		t.Fatalf("income total = %d", summary.IncomeTotalCents)
	}
	if summary.SpendingTotalCents != 2000 {
		t.Fatalf("spending total = %d", summary.SpendingTotalCents)
	}
	if summary.ExpectedBalanceCents != 598000 {
		t.Fatalf("expected balance = %d", summary.ExpectedBalanceCents)
	}
	if summary.VarianceCents != 0 {
		t.Fatalf("variance = %d", summary.VarianceCents)
	}
	if summary.TotalReservedCents != 88000 {
		t.Fatalf("total reserved = %d", summary.TotalReservedCents)
	}
	if summary.AvailableBalanceCents != 510000 {
		t.Fatalf("available balance = %d", summary.AvailableBalanceCents)
	}
	if summary.ReviewCounts.UnsortedCount != 1 || summary.ReviewCounts.RoundedCount != 1 {
		t.Fatalf("review counts = %#v", summary.ReviewCounts)
	}

	grabID := categoryIDByName(t, summary.Categories, "Grab")
	updated, err := repo.UpdateTransaction(ctx, transaction.ID, rawPatch(map[string]any{"category_id": grabID}))
	if err != nil {
		t.Fatal(err)
	}
	if updated.CategoryName != "Grab" {
		t.Fatalf("category after update = %q", updated.CategoryName)
	}

	summary, err = repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Allocations[0].SpentCents != 2000 || summary.Allocations[0].RemainingCents != 58000 {
		t.Fatalf("work allocation summary = %#v", summary.Allocations[0])
	}
	if summary.ReviewCounts.UnsortedCount != 0 || summary.ReviewCounts.RoundedCount != 1 {
		t.Fatalf("review counts after reclassify = %#v", summary.ReviewCounts)
	}
}

func TestWalletBalanceTracksIncomeAndTransactionDeltas(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 0,
		IncomeItems: []MonthPreviewIncomeItemRequest{{
			Name:           "Salary",
			AmountCents:    500000,
			AppliesToMonth: "2026-05",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	summary := mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 500000 || summary.ExpectedBalanceCents != 500000 || summary.VarianceCents != 0 {
		t.Fatalf("initial summary = %#v", summary)
	}

	work, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Work Expense", BudgetedCents: 50000})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: work.ID,
		AmountCents:  12500,
		Date:         "2026-05-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	summary = mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 487500 || summary.ExpectedBalanceCents != 487500 || summary.VarianceCents != 0 {
		t.Fatalf("summary after spend = %#v", summary)
	}

	if _, err := repo.UpdateMonth(ctx, "2026-05", rawPatch(map[string]any{"wallet_balance_cents": 480000})); err != nil {
		t.Fatal(err)
	}
	bonus, err := repo.CreateIncome(ctx, "2026-05", CreateIncomeRequest{
		Name:           "Bonus",
		AmountCents:    20000,
		AppliesToMonth: "2026-05",
	})
	if err != nil {
		t.Fatal(err)
	}
	summary = mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 500000 || summary.ExpectedBalanceCents != 507500 || summary.VarianceCents != -7500 {
		t.Fatalf("summary after manual baseline and bonus = %#v", summary)
	}

	if _, err := repo.UpdateIncome(ctx, bonus.ID, rawPatch(map[string]any{"amount_cents": 15000})); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateTransaction(ctx, transaction.ID, rawPatch(map[string]any{"amount_cents": 10000})); err != nil {
		t.Fatal(err)
	}
	summary = mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 497500 || summary.ExpectedBalanceCents != 505000 || summary.VarianceCents != -7500 {
		t.Fatalf("summary after edits = %#v", summary)
	}

	if err := repo.DeleteTransaction(ctx, transaction.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteIncome(ctx, bonus.ID); err != nil {
		t.Fatal(err)
	}
	summary = mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 492500 || summary.ExpectedBalanceCents != 500000 || summary.VarianceCents != -7500 {
		t.Fatalf("summary after deletes = %#v", summary)
	}
}

func TestWalletValidation(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{Month: "2026-13"}); err == nil {
		t.Fatal("expected invalid month error")
	}
	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{Month: "2026-05"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Work", BudgetedCents: -1}); err == nil {
		t.Fatal("expected negative allocation error")
	}
	if _, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{AmountCents: 100}); err == nil {
		t.Fatal("expected missing allocation error")
	}
}

func TestCreateMonthFromTemplatesAndCarryForward(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	settings, err := repo.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	emergency := allocationTemplateByName(t, settings.AllocationTemplates, "Emergency Fund")
	salary := incomeTemplateByName(t, settings.IncomeTemplates, "Salary")
	if _, err := repo.UpdateAllocationTemplate(ctx, emergency.ID, rawPatch(map[string]any{
		"default_amount_cents": 10000,
		"carry_forward":        true,
		"active":               true,
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateIncomeTemplate(ctx, salary.ID, rawPatch(map[string]any{
		"default_amount_cents": 500000,
		"default_day":          28,
		"active":               true,
	})); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 100000,
		WalletBalanceCents:  int64Ptr(600000),
		UseTemplates:        true,
		CarryForward:        true,
	}); err != nil {
		t.Fatal(err)
	}
	may, err := repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	emergencyAllocation := allocationByName(t, may.Allocations, "Emergency Fund")
	if emergencyAllocation.BudgetedCents != 10000 {
		t.Fatalf("may emergency budget = %d", emergencyAllocation.BudgetedCents)
	}
	if _, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: emergencyAllocation.ID,
		Date:         "2026-05-15",
		AmountCents:  2500,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-06",
		OpeningBalanceCents: 597500,
		WalletBalanceCents:  int64Ptr(597500),
		UseTemplates:        true,
		CarryForward:        true,
	}); err != nil {
		t.Fatal(err)
	}
	june, err := repo.Summary(ctx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	juneEmergency := allocationByName(t, june.Allocations, "Emergency Fund")
	if juneEmergency.BudgetedCents != 17500 {
		t.Fatalf("june emergency budget = %d", juneEmergency.BudgetedCents)
	}
	if len(june.IncomeItems) == 0 || june.IncomeItems[0].AmountCents != 500000 || june.IncomeItems[0].ReceivedDate == nil || *june.IncomeItems[0].ReceivedDate != "2026-06-28" {
		t.Fatalf("june income items = %#v", june.IncomeItems)
	}
	juneWork := allocationByName(t, june.Allocations, "Work Expense")
	if categoryIDByName(t, juneWork.DefaultCategories, "Office Supplies") != "wallet-category-office-supplies" {
		t.Fatalf("work defaults = %#v", juneWork.DefaultCategories)
	}
}

func TestPreviewMonthFromTemplatesAndConfirmReviewedRows(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	settings, err := repo.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workTemplate := allocationTemplateByName(t, settings.AllocationTemplates, "Work Expense")
	if categoryIDByName(t, workTemplate.DefaultCategories, "Office Supplies") != "wallet-category-office-supplies" {
		t.Fatalf("work template defaults = %#v", workTemplate.DefaultCategories)
	}
	salary := incomeTemplateByName(t, settings.IncomeTemplates, "Salary")
	if _, err := repo.UpdateIncomeTemplate(ctx, salary.ID, rawPatch(map[string]any{
		"default_amount_cents": 500000,
		"default_day":          25,
		"active":               true,
	})); err != nil {
		t.Fatal(err)
	}

	preview, err := repo.PreviewMonth(ctx, CreateMonthRequest{
		Month:               "2026-07",
		OpeningBalanceCents: 125000,
		WalletBalanceCents:  int64Ptr(125000),
		UseTemplates:        true,
		CarryForward:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetMonth(ctx, "2026-07"); err == nil {
		t.Fatal("preview wrote a wallet month")
	}
	if len(preview.IncomeItems) == 0 || len(preview.Allocations) == 0 {
		t.Fatalf("preview rows = %#v", preview)
	}
	work := allocationByNameRaw(t, preview.Allocations, "Work Expense")
	work.Name = "Client Work"
	work.BudgetedCents = 42000
	work.DefaultCategories = []Category{{ID: "wallet-category-coffee"}}

	req := CreateMonthRequest{
		Month:               preview.Month.Month,
		OpeningBalanceCents: preview.Month.OpeningBalanceCents,
		WalletBalanceCents:  &preview.Month.WalletBalanceCents,
		IncomeItems:         incomePreviewRequests(preview.IncomeItems),
		Allocations:         allocationPreviewRequests(replacePreviewAllocation(preview.Allocations, work)),
	}
	if _, err := repo.CreateMonth(ctx, req); err != nil {
		t.Fatal(err)
	}
	summary, err := repo.Summary(ctx, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	clientWork := allocationByName(t, summary.Allocations, "Client Work")
	if clientWork.BudgetedCents != 42000 || clientWork.TemplateID == nil || *clientWork.TemplateID != workTemplate.ID {
		t.Fatalf("client work allocation = %#v", clientWork)
	}
	if len(clientWork.DefaultCategories) != 1 || clientWork.DefaultCategories[0].ID != "wallet-category-coffee" {
		t.Fatalf("client work defaults = %#v", clientWork.DefaultCategories)
	}
	if len(summary.IncomeItems) == 0 || summary.IncomeItems[0].ReceivedDate == nil || *summary.IncomeItems[0].ReceivedDate != "2026-07-25" {
		t.Fatalf("reviewed income rows = %#v", summary.IncomeItems)
	}
}

func TestCategorySettingsProtectUnsorted(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	category, err := repo.CreateCategory(ctx, CreateCategoryRequest{Name: "Snacks", SortOrder: 999})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateCategory(ctx, category.ID, rawPatch(map[string]any{"name": "Office Snacks", "active": false}))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Office Snacks" || updated.Active {
		t.Fatalf("updated category = %#v", updated)
	}
	if err := repo.DeleteCategory(ctx, UnsortedCategoryID); err == nil {
		t.Fatal("expected unsorted delete error")
	}
	if _, err := repo.UpdateCategory(ctx, UnsortedCategoryID, rawPatch(map[string]any{"active": false})); err == nil {
		t.Fatal("expected unsorted deactivate error")
	}
}

func TestSettingsOrderFollowsSortOrderAcrossActiveStates(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	settings, err := repo.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	emergency := allocationTemplateByName(t, settings.AllocationTemplates, "Emergency Fund")
	if _, err := repo.UpdateAllocationTemplate(ctx, emergency.ID, rawPatch(map[string]any{
		"active":     false,
		"sort_order": -10,
	})); err != nil {
		t.Fatal(err)
	}
	freelance := incomeTemplateByName(t, settings.IncomeTemplates, "Freelance")
	if _, err := repo.UpdateIncomeTemplate(ctx, freelance.ID, rawPatch(map[string]any{
		"active":     false,
		"sort_order": -10,
	})); err != nil {
		t.Fatal(err)
	}
	foodID := categoryIDByName(t, settings.Categories, "Food")
	if _, err := repo.UpdateCategory(ctx, foodID, rawPatch(map[string]any{
		"active":     false,
		"sort_order": -10,
	})); err != nil {
		t.Fatal(err)
	}

	settings, err = repo.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AllocationTemplates[0].ID != emergency.ID {
		t.Fatalf("allocation order starts with %q, want %q", settings.AllocationTemplates[0].Name, emergency.Name)
	}
	if settings.IncomeTemplates[0].ID != freelance.ID {
		t.Fatalf("income order starts with %q, want %q", settings.IncomeTemplates[0].Name, freelance.Name)
	}
	if settings.Categories[0].ID != foodID {
		t.Fatalf("category order starts with %q, want Food", settings.Categories[0].Name)
	}
}

func TestReconciliationBalanceUpdateCreatesHiddenLedgerTransaction(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 100000,
		WalletBalanceCents:  int64Ptr(100000),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateIncome(ctx, "2026-05", CreateIncomeRequest{
		Name:           "Salary",
		AmountCents:    500000,
		AppliesToMonth: "2026-05",
	}); err != nil {
		t.Fatal(err)
	}
	work, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Work Expense", BudgetedCents: 50000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: work.ID,
		AmountCents:  1000,
		Date:         "2026-05-10",
		Rounded:      true,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := repo.CreateBalanceUpdate(ctx, "2026-05", CreateBalanceUpdateRequest{
		NewBalanceCents:  599100,
		CreateAdjustment: true,
		AdjustmentReason: "rounding",
		AdjustmentNote:   stringPtr("rounded cash difference"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BalanceUpdate.PreviousBalanceCents != 599000 || result.BalanceUpdate.ExpectedBalanceCents != 599000 {
		t.Fatalf("balance update = %#v", result.BalanceUpdate)
	}
	if result.Adjustment != nil {
		t.Fatalf("legacy adjustment = %#v", result.Adjustment)
	}
	if result.Transaction == nil || result.Transaction.Kind != "income" || result.Transaction.AmountCents != 100 ||
		result.Transaction.AllocationName != ReconciliationAllocationName || result.Transaction.CategoryName != ReconciliationCategoryName {
		t.Fatalf("reconciliation income transaction = %#v", result.Transaction)
	}
	summary, err := repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if summary.WalletBalanceCents != 599100 || summary.ExpectedBalanceCents != 599100 ||
		summary.IncomeTotalCents != 500100 || summary.SpendingTotalCents != 1000 ||
		summary.AdjustmentTotalCents != 0 || summary.VarianceCents != 0 {
		t.Fatalf("summary after reconciliation = %#v", summary)
	}
	if len(summary.Allocations) != 1 || summary.Allocations[0].Name != "Work Expense" {
		t.Fatalf("visible allocations = %#v", summary.Allocations)
	}
	if len(summary.BalanceUpdates) != 1 || len(summary.Adjustments) != 0 {
		t.Fatalf("reconciliation history = updates:%#v adjustments:%#v", summary.BalanceUpdates, summary.Adjustments)
	}
	settings, err := repo.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range settings.Categories {
		if category.Name == ReconciliationCategoryName || category.SystemKey != nil && *category.SystemKey == ReconciliationCategorySystemKey {
			t.Fatalf("hidden adjustment category leaked into settings: %#v", settings.Categories)
		}
	}

	result, err = repo.CreateBalanceUpdate(ctx, "2026-05", CreateBalanceUpdateRequest{
		NewBalanceCents: 598900,
		Note:            stringPtr("cash count lower"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Transaction == nil || result.Transaction.Kind != "spend" || result.Transaction.AmountCents != 200 ||
		result.Transaction.AllocationName != ReconciliationAllocationName || result.Transaction.CategoryName != ReconciliationCategoryName {
		t.Fatalf("reconciliation expense transaction = %#v", result.Transaction)
	}
	summary = mustSummary(t, repo, ctx, "2026-05")
	if summary.WalletBalanceCents != 598900 || summary.ExpectedBalanceCents != 598900 ||
		summary.IncomeTotalCents != 500100 || summary.SpendingTotalCents != 1200 || summary.VarianceCents != 0 {
		t.Fatalf("summary after expense reconciliation = %#v", summary)
	}
}

func TestMonthBookAndDeleteMonthCascade(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	month, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateIncome(ctx, "2026-05", CreateIncomeRequest{
		Name:           "Salary",
		AmountCents:    500000,
		AppliesToMonth: "2026-05",
	}); err != nil {
		t.Fatal(err)
	}
	work, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{
		Name:               "Work Expense",
		BudgetedCents:      50000,
		DefaultCategoryIDs: []string{"wallet-category-office-supplies"},
	})
	if err != nil {
		t.Fatal(err)
	}
	food, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Food", BudgetedCents: 30000})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: work.ID,
		AmountCents:  5000,
		Date:         "2026-05-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SplitTransaction(ctx, parent.ID, CreateTransactionSplitRequest{Splits: []TransactionSplitInput{
		{AllocationID: work.ID, CategoryID: "wallet-category-office-supplies", AmountCents: 2000, Note: stringPtr("supplies")},
		{AllocationID: food.ID, CategoryID: "wallet-category-food", AmountCents: 3000, Note: stringPtr("meal")},
	}}); err != nil {
		t.Fatal(err)
	}
	update, err := repo.CreateBalanceUpdate(ctx, "2026-05", CreateBalanceUpdateRequest{NewBalanceCents: 595100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateReconciliationAdjustment(ctx, "2026-05", CreateReconciliationAdjustmentRequest{
		AmountCents:     100,
		Reason:          "manual_correction",
		BalanceUpdateID: &update.BalanceUpdate.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CloseMonth(ctx, "2026-05"); err != nil {
		t.Fatal(err)
	}

	book, err := repo.ListMonthBook(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(book) != 1 {
		t.Fatalf("month book rows = %#v", book)
	}
	row := book[0]
	if row.Month != "2026-05" || row.Status != "closed" || row.AllocationCount != 2 || row.TransactionCount != 3 {
		t.Fatalf("month book row = %#v", row)
	}
	if row.IncomeTotalCents != 500100 || row.SpendingTotalCents != 5000 ||
		row.AdjustmentTotalCents != 100 || row.ExpectedBalanceCents != 595100 || row.VarianceCents != 0 {
		t.Fatalf("month book totals = %#v", row)
	}

	if err := repo.DeleteMonth(ctx, "2026-05"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetMonth(ctx, "2026-05"); err == nil {
		t.Fatal("expected deleted month to be missing")
	}
	assertCount(t, repo, "wallet_income_items", "month_id", month.ID, 0)
	assertCount(t, repo, "wallet_allocations", "month_id", month.ID, 0)
	assertCount(t, repo, "wallet_transactions", "month_id", month.ID, 0)
	assertCount(t, repo, "wallet_balance_updates", "month_id", month.ID, 0)
	assertCount(t, repo, "wallet_reconciliation_adjustments", "month_id", month.ID, 0)
	assertCount(t, repo, "wallet_allocation_default_categories", "allocation_id", work.ID, 0)
	assertCount(t, repo, "wallet_transaction_splits", "parent_transaction_id", parent.ID, 0)
}

func TestReviewReportsCloseAndSplitFlow(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{
		Month:               "2026-05",
		OpeningBalanceCents: 100000,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateIncome(ctx, "2026-05", CreateIncomeRequest{
		Name:           "Salary",
		AmountCents:    500000,
		AppliesToMonth: "2026-05",
	}); err != nil {
		t.Fatal(err)
	}
	work, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Work Expense", BudgetedCents: 50000})
	if err != nil {
		t.Fatal(err)
	}
	food, err := repo.CreateAllocation(ctx, "2026-05", CreateAllocationRequest{Name: "Food", BudgetedCents: 30000})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{
		AllocationID: work.ID,
		AmountCents:  5000,
		Date:         "2026-05-10",
		Note:         stringPtr("office day"),
	})
	if err != nil {
		t.Fatal(err)
	}

	review, err := repo.ReviewTransactions(ctx, "2026-05", ReviewFilters{CleanupOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Transactions) != 1 || review.Transactions[0].ID != parent.ID {
		t.Fatalf("review transactions = %#v", review.Transactions)
	}
	detail, err := repo.AllocationDetail(ctx, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Allocation.SpentCents != 5000 || len(detail.CategoryBreakdown) != 1 || detail.CategoryBreakdown[0].CategoryName != "Unsorted" {
		t.Fatalf("allocation detail = %#v", detail)
	}

	lunchID := categoryIDByName(t, mustSummary(t, repo, ctx, "2026-05").Categories, "Lunch")
	grabID := categoryIDByName(t, mustSummary(t, repo, ctx, "2026-05").Categories, "Grab")
	split, err := repo.SplitTransaction(ctx, parent.ID, CreateTransactionSplitRequest{Splits: []TransactionSplitInput{
		{AllocationID: work.ID, CategoryID: lunchID, AmountCents: 2000, Note: stringPtr("lunch")},
		{AllocationID: food.ID, CategoryID: grabID, AmountCents: 3000, Note: stringPtr("grab")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(split.Children) != 2 {
		t.Fatalf("split = %#v", split)
	}
	summary, err := repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if summary.SpendingTotalCents != 5000 || summary.ExpectedBalanceCents != 595000 || summary.VarianceCents != 0 {
		t.Fatalf("summary after split = %#v", summary)
	}
	workDetail, err := repo.AllocationDetail(ctx, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workDetail.Allocation.SpentCents != 2000 {
		t.Fatalf("work spent after split = %#v", workDetail.Allocation)
	}
	categoryReport, err := repo.CategoryReport(ctx, "2026-05", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(categoryReport) != 2 {
		t.Fatalf("category report = %#v", categoryReport)
	}
	allocationReport, err := repo.AllocationReport(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(allocationReport) != 2 {
		t.Fatalf("allocation report = %#v", allocationReport)
	}
	monthlyReport, err := repo.MonthlyReport(ctx, "2026-05", "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if len(monthlyReport) != 1 || monthlyReport[0].SpendingTotalCents != 5000 {
		t.Fatalf("monthly report = %#v", monthlyReport)
	}
	if err := repo.DeleteTransaction(ctx, split.Children[0].ID); err == nil {
		t.Fatal("expected split child delete to fail")
	}
	if err := repo.DeleteTransaction(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	summary, err = repo.Summary(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if summary.SpendingTotalCents != 0 {
		t.Fatalf("summary after split parent delete = %#v", summary)
	}
	closed, err := repo.CloseMonth(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "closed" || closed.ClosedWalletBalanceCents == nil || *closed.ClosedWalletBalanceCents != 600000 {
		t.Fatalf("closed month = %#v", closed)
	}
	if _, err := repo.CreateTransaction(ctx, "2026-05", CreateTransactionRequest{AllocationID: work.ID, AmountCents: 100, Date: "2026-05-11"}); err == nil {
		t.Fatal("expected closed month mutation to fail")
	}
	if _, err := repo.UpdateAllocation(ctx, work.ID, rawPatch(map[string]any{"budgeted_cents": 75000})); err == nil {
		t.Fatal("expected closed month allocation edit to fail")
	}
	reopened, err := repo.ReopenMonth(ctx, "2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != "open" || reopened.ClosedAt != nil {
		t.Fatalf("reopened month = %#v", reopened)
	}
}

func TestAllocationDetailEditsUpdateSummary(t *testing.T) {
	repo, cleanup := newTestRepository(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := repo.CreateMonth(ctx, CreateMonthRequest{Month: "2026-08"}); err != nil {
		t.Fatal(err)
	}
	allocation, err := repo.CreateAllocation(ctx, "2026-08", CreateAllocationRequest{
		Name:               "Work Expense",
		BudgetedCents:      10000,
		Type:               "flexible",
		DefaultCategoryIDs: []string{"wallet-category-office-supplies"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTransaction(ctx, "2026-08", CreateTransactionRequest{
		AllocationID: allocation.ID,
		CategoryID:   "wallet-category-office-supplies",
		Date:         "2026-08-01",
		AmountCents:  2500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAllocation(ctx, allocation.ID, rawPatch(map[string]any{
		"name":           "Client Work",
		"budgeted_cents": 15000,
		"type":           "fixed",
		"carry_forward":  true,
		"active":         true,
	})); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.AllocationDetail(ctx, allocation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Allocation.Name != "Client Work" || detail.Allocation.BudgetedCents != 15000 || detail.Allocation.RemainingCents != 12500 || detail.Allocation.Type != "fixed" || !detail.Allocation.CarryForward {
		t.Fatalf("allocation detail after edit = %#v", detail.Allocation)
	}
	if len(detail.Allocation.DefaultCategories) != 1 || detail.Allocation.DefaultCategories[0].ID != "wallet-category-office-supplies" {
		t.Fatalf("allocation defaults = %#v", detail.Allocation.DefaultCategories)
	}
}

func newTestRepository(t *testing.T) (*Repository, func()) {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Path:          filepath.Join(t.TempDir(), "wallet.db"),
		MigrationsDir: filepath.Join("..", "..", "migrations"),
		AppEnv:        "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(database), func() { _ = database.Close() }
}

func categoryIDByName(t *testing.T, categories []Category, name string) string {
	t.Helper()
	for _, category := range categories {
		if category.Name == name {
			return category.ID
		}
	}
	t.Fatalf("category %q not found in %#v", name, categories)
	return ""
}

func stringPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func assertCount(t *testing.T, repo *Repository, table string, column string, value string, want int) {
	t.Helper()
	var got int
	query := "SELECT COUNT(*) FROM " + table + " WHERE " + column + " = ?"
	if err := repo.db.QueryRowContext(context.Background(), query, value).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count for %s = %d, want %d", table, value, got, want)
	}
}

func rawPatch(value map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(value))
	for key, item := range value {
		raw, _ := json.Marshal(item)
		out[key] = raw
	}
	return out
}

func allocationTemplateByName(t *testing.T, templates []AllocationTemplate, name string) AllocationTemplate {
	t.Helper()
	for _, template := range templates {
		if template.Name == name {
			return template
		}
	}
	t.Fatalf("allocation template %q not found in %#v", name, templates)
	return AllocationTemplate{}
}

func incomeTemplateByName(t *testing.T, templates []IncomeTemplate, name string) IncomeTemplate {
	t.Helper()
	for _, template := range templates {
		if template.Name == name {
			return template
		}
	}
	t.Fatalf("income template %q not found in %#v", name, templates)
	return IncomeTemplate{}
}

func allocationByName(t *testing.T, allocations []AllocationSummary, name string) AllocationSummary {
	t.Helper()
	for _, allocation := range allocations {
		if allocation.Name == name {
			return allocation
		}
	}
	t.Fatalf("allocation %q not found in %#v", name, allocations)
	return AllocationSummary{}
}

func allocationByNameRaw(t *testing.T, allocations []Allocation, name string) Allocation {
	t.Helper()
	for _, allocation := range allocations {
		if allocation.Name == name {
			return allocation
		}
	}
	t.Fatalf("allocation %q not found in %#v", name, allocations)
	return Allocation{}
}

func replacePreviewAllocation(allocations []Allocation, replacement Allocation) []Allocation {
	out := append([]Allocation{}, allocations...)
	for i := range out {
		if out[i].ID == replacement.ID {
			out[i] = replacement
			return out
		}
	}
	return append(out, replacement)
}

func incomePreviewRequests(items []IncomeItem) []MonthPreviewIncomeItemRequest {
	out := make([]MonthPreviewIncomeItemRequest, 0, len(items))
	for _, item := range items {
		out = append(out, MonthPreviewIncomeItemRequest{
			Name:           item.Name,
			AmountCents:    item.AmountCents,
			ReceivedDate:   item.ReceivedDate,
			AppliesToMonth: item.AppliesToMonth,
			Notes:          item.Notes,
		})
	}
	return out
}

func allocationPreviewRequests(allocations []Allocation) []MonthPreviewAllocationRequest {
	out := make([]MonthPreviewAllocationRequest, 0, len(allocations))
	for _, allocation := range allocations {
		active := allocation.Active
		out = append(out, MonthPreviewAllocationRequest{
			TemplateID:         allocation.TemplateID,
			Name:               allocation.Name,
			BudgetedCents:      allocation.BudgetedCents,
			Type:               allocation.Type,
			CarryForward:       allocation.CarryForward,
			SortOrder:          allocation.SortOrder,
			Active:             &active,
			DefaultCategoryIDs: categoryIDs(allocation.DefaultCategories),
		})
	}
	return out
}

func mustSummary(t *testing.T, repo *Repository, ctx context.Context, month string) MonthSummary {
	t.Helper()
	summary, err := repo.Summary(ctx, month)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}
