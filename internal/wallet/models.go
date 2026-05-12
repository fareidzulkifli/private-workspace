package wallet

type Month struct {
	ID                       string  `json:"id"`
	Month                    string  `json:"month"`
	OpeningBalanceCents      int64   `json:"opening_balance_cents"`
	WalletBalanceCents       int64   `json:"wallet_balance_cents"`
	Status                   string  `json:"status"`
	ClosedAt                 *string `json:"closed_at"`
	ClosedWalletBalanceCents *int64  `json:"closed_wallet_balance_cents"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
}

type IncomeItem struct {
	ID             string  `json:"id"`
	MonthID        string  `json:"month_id"`
	Name           string  `json:"name"`
	AmountCents    int64   `json:"amount_cents"`
	ReceivedDate   *string `json:"received_date"`
	AppliesToMonth string  `json:"applies_to_month"`
	Notes          *string `json:"notes"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type Allocation struct {
	ID                string     `json:"id"`
	MonthID           string     `json:"month_id"`
	TemplateID        *string    `json:"template_id"`
	Name              string     `json:"name"`
	BudgetedCents     int64      `json:"budgeted_cents"`
	Type              string     `json:"type"`
	CarryForward      bool       `json:"carry_forward"`
	SortOrder         int        `json:"sort_order"`
	Active            bool       `json:"active"`
	DefaultCategories []Category `json:"default_categories"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
}

type AllocationSummary struct {
	Allocation
	SpentCents     int64 `json:"spent_cents"`
	RemainingCents int64 `json:"remaining_cents"`
}

type Category struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	SystemKey *string `json:"system_key"`
	Active    bool    `json:"active"`
	SortOrder int     `json:"sort_order"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type Transaction struct {
	ID                  string  `json:"id"`
	MonthID             string  `json:"month_id"`
	AllocationID        string  `json:"allocation_id"`
	AllocationName      string  `json:"allocation_name,omitempty"`
	CategoryID          string  `json:"category_id"`
	CategoryName        string  `json:"category_name,omitempty"`
	Date                string  `json:"date"`
	AmountCents         int64   `json:"amount_cents"`
	Note                *string `json:"note"`
	Rounded             bool    `json:"rounded"`
	Kind                string  `json:"kind"`
	Source              string  `json:"source"`
	ParentTransactionID *string `json:"parent_transaction_id"`
	SplitRole           string  `json:"split_role,omitempty"`
	SplitChildCount     int     `json:"split_child_count,omitempty"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

type ReviewCounts struct {
	UnsortedCount int   `json:"unsorted_count"`
	UnsortedCents int64 `json:"unsorted_cents"`
	RoundedCount  int   `json:"rounded_count"`
	RoundedCents  int64 `json:"rounded_cents"`
}

type MonthSummary struct {
	Month                 Month                      `json:"month"`
	IncomeItems           []IncomeItem               `json:"income_items"`
	Allocations           []AllocationSummary        `json:"allocations"`
	RecentTransactions    []Transaction              `json:"recent_transactions"`
	Categories            []Category                 `json:"categories"`
	BalanceUpdates        []BalanceUpdate            `json:"balance_updates"`
	Adjustments           []ReconciliationAdjustment `json:"adjustments"`
	ReviewCounts          ReviewCounts               `json:"review_counts"`
	IncomeTotalCents      int64                      `json:"income_total_cents"`
	SpendingTotalCents    int64                      `json:"spending_total_cents"`
	AdjustmentTotalCents  int64                      `json:"adjustment_total_cents"`
	ExpectedBalanceCents  int64                      `json:"expected_balance_cents"`
	WalletBalanceCents    int64                      `json:"wallet_balance_cents"`
	VarianceCents         int64                      `json:"variance_cents"`
	TotalReservedCents    int64                      `json:"total_reserved_cents"`
	AvailableBalanceCents int64                      `json:"available_balance_cents"`
}

type MonthBookRow struct {
	ID                       string  `json:"id"`
	Month                    string  `json:"month"`
	Status                   string  `json:"status"`
	OpeningBalanceCents      int64   `json:"opening_balance_cents"`
	WalletBalanceCents       int64   `json:"wallet_balance_cents"`
	ClosedAt                 *string `json:"closed_at"`
	ClosedWalletBalanceCents *int64  `json:"closed_wallet_balance_cents"`
	CreatedAt                string  `json:"created_at"`
	UpdatedAt                string  `json:"updated_at"`
	IncomeTotalCents         int64   `json:"income_total_cents"`
	SpendingTotalCents       int64   `json:"spending_total_cents"`
	AdjustmentTotalCents     int64   `json:"adjustment_total_cents"`
	ExpectedBalanceCents     int64   `json:"expected_balance_cents"`
	VarianceCents            int64   `json:"variance_cents"`
	TotalReservedCents       int64   `json:"total_reserved_cents"`
	AvailableBalanceCents    int64   `json:"available_balance_cents"`
	AllocationCount          int     `json:"allocation_count"`
	TransactionCount         int     `json:"transaction_count"`
}

type BalanceUpdate struct {
	ID                   string  `json:"id"`
	MonthID              string  `json:"month_id"`
	PreviousBalanceCents int64   `json:"previous_balance_cents"`
	NewBalanceCents      int64   `json:"new_balance_cents"`
	ExpectedBalanceCents int64   `json:"expected_balance_cents"`
	DifferenceCents      int64   `json:"difference_cents"`
	Note                 *string `json:"note"`
	CreatedAt            string  `json:"created_at"`
}

type ReconciliationAdjustment struct {
	ID              string  `json:"id"`
	MonthID         string  `json:"month_id"`
	BalanceUpdateID *string `json:"balance_update_id"`
	AmountCents     int64   `json:"amount_cents"`
	Reason          string  `json:"reason"`
	Note            *string `json:"note"`
	CreatedAt       string  `json:"created_at"`
}

type BalanceUpdateResult struct {
	BalanceUpdate BalanceUpdate             `json:"balance_update"`
	Adjustment    *ReconciliationAdjustment `json:"adjustment"`
	Transaction   *Transaction              `json:"transaction,omitempty"`
}

type ReviewTransactionsResult struct {
	Transactions []Transaction `json:"transactions"`
}

type CategoryBreakdownRow struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	AmountCents  int64  `json:"amount_cents"`
	Count        int    `json:"count"`
}

type AllocationDetail struct {
	Allocation        AllocationSummary      `json:"allocation"`
	CategoryBreakdown []CategoryBreakdownRow `json:"category_breakdown"`
	Transactions      []Transaction          `json:"transactions"`
}

type MonthlyReportRow struct {
	Month                 string `json:"month"`
	Status                string `json:"status"`
	OpeningBalanceCents   int64  `json:"opening_balance_cents"`
	IncomeTotalCents      int64  `json:"income_total_cents"`
	SpendingTotalCents    int64  `json:"spending_total_cents"`
	AdjustmentTotalCents  int64  `json:"adjustment_total_cents"`
	ExpectedBalanceCents  int64  `json:"expected_balance_cents"`
	WalletBalanceCents    int64  `json:"wallet_balance_cents"`
	VarianceCents         int64  `json:"variance_cents"`
	TotalReservedCents    int64  `json:"total_reserved_cents"`
	AvailableBalanceCents int64  `json:"available_balance_cents"`
}

type CategoryReportRow struct {
	CategoryID     string  `json:"category_id"`
	CategoryName   string  `json:"category_name"`
	AmountCents    int64   `json:"amount_cents"`
	Count          int     `json:"count"`
	PercentOfSpend float64 `json:"percent_of_spend"`
}

type ReviewReport struct {
	Month                string       `json:"month"`
	ReviewCounts         ReviewCounts `json:"review_counts"`
	VarianceCents        int64        `json:"variance_cents"`
	AdjustmentTotalCents int64        `json:"adjustment_total_cents"`
}

type CreateMonthRequest struct {
	Month               string                          `json:"month"`
	OpeningBalanceCents int64                           `json:"opening_balance_cents"`
	WalletBalanceCents  *int64                          `json:"wallet_balance_cents"`
	UseTemplates        bool                            `json:"use_templates"`
	CarryForward        bool                            `json:"carry_forward"`
	IncomeItems         []MonthPreviewIncomeItemRequest `json:"income_items"`
	Allocations         []MonthPreviewAllocationRequest `json:"allocations"`
}

type MonthPreview struct {
	Month       Month        `json:"month"`
	IncomeItems []IncomeItem `json:"income_items"`
	Allocations []Allocation `json:"allocations"`
	Categories  []Category   `json:"categories"`
	Source      string       `json:"source"`
}

type MonthPreviewIncomeItemRequest struct {
	Name           string  `json:"name"`
	AmountCents    int64   `json:"amount_cents"`
	ReceivedDate   *string `json:"received_date"`
	AppliesToMonth string  `json:"applies_to_month"`
	Notes          *string `json:"notes"`
}

type MonthPreviewAllocationRequest struct {
	TemplateID         *string  `json:"template_id"`
	Name               string   `json:"name"`
	BudgetedCents      int64    `json:"budgeted_cents"`
	Type               string   `json:"type"`
	CarryForward       bool     `json:"carry_forward"`
	SortOrder          int      `json:"sort_order"`
	Active             *bool    `json:"active"`
	DefaultCategoryIDs []string `json:"default_category_ids"`
}

type AllocationTemplate struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	DefaultAmountCents int64      `json:"default_amount_cents"`
	Type               string     `json:"type"`
	CarryForward       bool       `json:"carry_forward"`
	Active             bool       `json:"active"`
	SortOrder          int        `json:"sort_order"`
	DefaultCategories  []Category `json:"default_categories"`
	CreatedAt          string     `json:"created_at"`
	UpdatedAt          string     `json:"updated_at"`
}

type IncomeTemplate struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DefaultAmountCents int64  `json:"default_amount_cents"`
	DefaultDay         *int   `json:"default_day"`
	Active             bool   `json:"active"`
	SortOrder          int    `json:"sort_order"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type Settings struct {
	AllocationTemplates []AllocationTemplate `json:"allocation_templates"`
	IncomeTemplates     []IncomeTemplate     `json:"income_templates"`
	Categories          []Category           `json:"categories"`
}

type CreateAllocationTemplateRequest struct {
	Name               string   `json:"name"`
	DefaultAmountCents int64    `json:"default_amount_cents"`
	Type               string   `json:"type"`
	CarryForward       bool     `json:"carry_forward"`
	Active             *bool    `json:"active"`
	SortOrder          int      `json:"sort_order"`
	DefaultCategoryIDs []string `json:"default_category_ids"`
}

type CreateIncomeTemplateRequest struct {
	Name               string `json:"name"`
	DefaultAmountCents int64  `json:"default_amount_cents"`
	DefaultDay         *int   `json:"default_day"`
	Active             *bool  `json:"active"`
	SortOrder          int    `json:"sort_order"`
}

type CreateCategoryRequest struct {
	Name      string `json:"name"`
	Active    *bool  `json:"active"`
	SortOrder int    `json:"sort_order"`
}

type CreateIncomeRequest struct {
	Name           string  `json:"name"`
	AmountCents    int64   `json:"amount_cents"`
	ReceivedDate   *string `json:"received_date"`
	AppliesToMonth string  `json:"applies_to_month"`
	Notes          *string `json:"notes"`
}

type CreateAllocationRequest struct {
	TemplateID         *string  `json:"template_id"`
	Name               string   `json:"name"`
	BudgetedCents      int64    `json:"budgeted_cents"`
	Type               string   `json:"type"`
	CarryForward       bool     `json:"carry_forward"`
	SortOrder          int      `json:"sort_order"`
	Active             *bool    `json:"active"`
	DefaultCategoryIDs []string `json:"default_category_ids"`
}

type CreateTransactionRequest struct {
	AllocationID string  `json:"allocation_id"`
	CategoryID   string  `json:"category_id"`
	Date         string  `json:"date"`
	AmountCents  int64   `json:"amount_cents"`
	Note         *string `json:"note"`
	Rounded      bool    `json:"rounded"`
}

type CreateBalanceUpdateRequest struct {
	NewBalanceCents  int64   `json:"new_balance_cents"`
	Note             *string `json:"note"`
	CreateAdjustment bool    `json:"create_adjustment"`
	AdjustmentReason string  `json:"adjustment_reason"`
	AdjustmentNote   *string `json:"adjustment_note"`
}

type CreateReconciliationAdjustmentRequest struct {
	AmountCents     int64   `json:"amount_cents"`
	Reason          string  `json:"reason"`
	Note            *string `json:"note"`
	BalanceUpdateID *string `json:"balance_update_id"`
}

type TransactionSplitInput struct {
	AllocationID string  `json:"allocation_id"`
	CategoryID   string  `json:"category_id"`
	Date         string  `json:"date"`
	AmountCents  int64   `json:"amount_cents"`
	Note         *string `json:"note"`
	Rounded      bool    `json:"rounded"`
}

type CreateTransactionSplitRequest struct {
	Splits []TransactionSplitInput `json:"splits"`
}

type TransactionSplitResult struct {
	Parent   Transaction   `json:"parent"`
	Children []Transaction `json:"children"`
}

type TransactionSplitDetail struct {
	Parent   Transaction   `json:"parent"`
	Children []Transaction `json:"children"`
}
