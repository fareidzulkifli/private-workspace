package wallet

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/db"
	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Handler struct {
	repo *Repository
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{repo: NewRepository(database)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/wallet/months", h.ListMonths)
	r.Get("/api/wallet/months/book", h.ListMonthBook)
	r.Post("/api/wallet/months", h.CreateMonth)
	r.Post("/api/wallet/months/preview", h.PreviewMonth)
	r.Get("/api/wallet/months/{month}/summary", h.Summary)
	r.Get("/api/wallet/months/{month}/review", h.ReviewTransactions)
	r.Get("/api/wallet/months/{month}", h.GetMonth)
	r.Patch("/api/wallet/months/{month}", h.UpdateMonth)
	r.Delete("/api/wallet/months/{month}", h.DeleteMonth)
	r.Post("/api/wallet/months/{month}/close", h.CloseMonth)
	r.Post("/api/wallet/months/{month}/reopen", h.ReopenMonth)
	r.Get("/api/wallet/months/{month}/balance-updates", h.ListBalanceUpdates)
	r.Post("/api/wallet/months/{month}/balance-updates", h.CreateBalanceUpdate)
	r.Get("/api/wallet/months/{month}/reconciliation-adjustments", h.ListReconciliationAdjustments)
	r.Post("/api/wallet/months/{month}/reconciliation-adjustments", h.CreateReconciliationAdjustment)
	r.Get("/api/wallet/settings", h.Settings)
	r.Get("/api/wallet/reports/monthly", h.MonthlyReport)
	r.Get("/api/wallet/reports/categories", h.CategoryReport)
	r.Get("/api/wallet/reports/allocations", h.AllocationReport)
	r.Get("/api/wallet/reports/review", h.ReviewReport)

	r.Post("/api/wallet/months/{month}/income", h.CreateIncome)
	r.Patch("/api/wallet/income/{id}", h.UpdateIncome)
	r.Delete("/api/wallet/income/{id}", h.DeleteIncome)

	r.Post("/api/wallet/months/{month}/allocations", h.CreateAllocation)
	r.Get("/api/wallet/allocations/{id}/detail", h.AllocationDetail)
	r.Patch("/api/wallet/allocations/{id}", h.UpdateAllocation)
	r.Delete("/api/wallet/allocations/{id}", h.DeleteAllocation)

	r.Post("/api/wallet/months/{month}/transactions", h.CreateTransaction)
	r.Get("/api/wallet/transactions/{id}/split", h.SplitTransactionDetail)
	r.Post("/api/wallet/transactions/{id}/split", h.SplitTransaction)
	r.Patch("/api/wallet/transactions/{id}", h.UpdateTransaction)
	r.Delete("/api/wallet/transactions/{id}", h.DeleteTransaction)

	r.Post("/api/wallet/allocation-templates", h.CreateAllocationTemplate)
	r.Patch("/api/wallet/allocation-templates/{id}", h.UpdateAllocationTemplate)
	r.Delete("/api/wallet/allocation-templates/{id}", h.DeleteAllocationTemplate)

	r.Post("/api/wallet/income-templates", h.CreateIncomeTemplate)
	r.Patch("/api/wallet/income-templates/{id}", h.UpdateIncomeTemplate)
	r.Delete("/api/wallet/income-templates/{id}", h.DeleteIncomeTemplate)

	r.Post("/api/wallet/categories", h.CreateCategory)
	r.Patch("/api/wallet/categories/{id}", h.UpdateCategory)
	r.Delete("/api/wallet/categories/{id}", h.DeleteCategory)
}

func (h *Handler) ListMonths(w http.ResponseWriter, r *http.Request) {
	months, err := h.repo.ListMonths(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, months)
}

func (h *Handler) ListMonthBook(w http.ResponseWriter, r *http.Request) {
	book, err := h.repo.ListMonthBook(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, book)
}

func (h *Handler) CreateMonth(w http.ResponseWriter, r *http.Request) {
	var req CreateMonthRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	month, err := h.repo.CreateMonth(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, month)
}

func (h *Handler) PreviewMonth(w http.ResponseWriter, r *http.Request) {
	var req CreateMonthRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	preview, err := h.repo.PreviewMonth(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, preview)
}

func (h *Handler) GetMonth(w http.ResponseWriter, r *http.Request) {
	month, err := h.repo.GetMonth(r.Context(), chi.URLParam(r, "month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, month)
}

func (h *Handler) UpdateMonth(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	month, err := h.repo.UpdateMonth(r.Context(), chi.URLParam(r, "month"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, month)
}

func (h *Handler) DeleteMonth(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteMonth(r.Context(), chi.URLParam(r, "month")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.repo.Summary(r.Context(), chi.URLParam(r, "month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}

func (h *Handler) ReviewTransactions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	result, err := h.repo.ReviewTransactions(r.Context(), chi.URLParam(r, "month"), ReviewFilters{
		CleanupOnly:  query.Get("cleanup") != "false",
		UnsortedOnly: query.Get("unsorted") == "true",
		RoundedOnly:  query.Get("rounded") == "true",
		MissingNote:  query.Get("missing_note") == "true",
		AllocationID: query.Get("allocation_id"),
		CategoryID:   query.Get("category_id"),
		Limit:        parsePositiveInt(query.Get("limit"), 200),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CloseMonth(w http.ResponseWriter, r *http.Request) {
	month, err := h.repo.CloseMonth(r.Context(), chi.URLParam(r, "month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, month)
}

func (h *Handler) ReopenMonth(w http.ResponseWriter, r *http.Request) {
	month, err := h.repo.ReopenMonth(r.Context(), chi.URLParam(r, "month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, month)
}

func (h *Handler) ListBalanceUpdates(w http.ResponseWriter, r *http.Request) {
	updates, err := h.repo.ListBalanceUpdates(r.Context(), chi.URLParam(r, "month"), parsePositiveInt(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, updates)
}

func (h *Handler) CreateBalanceUpdate(w http.ResponseWriter, r *http.Request) {
	var req CreateBalanceUpdateRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	result, err := h.repo.CreateBalanceUpdate(r.Context(), chi.URLParam(r, "month"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) ListReconciliationAdjustments(w http.ResponseWriter, r *http.Request) {
	adjustments, err := h.repo.ListReconciliationAdjustments(r.Context(), chi.URLParam(r, "month"), parsePositiveInt(r.URL.Query().Get("limit"), 50))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, adjustments)
}

func (h *Handler) CreateReconciliationAdjustment(w http.ResponseWriter, r *http.Request) {
	var req CreateReconciliationAdjustmentRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	adjustment, err := h.repo.CreateReconciliationAdjustment(r.Context(), chi.URLParam(r, "month"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, adjustment)
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.Settings(r.Context())
	if err != nil {
		shared.WriteDBError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, settings)
}

func (h *Handler) MonthlyReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.repo.MonthlyReport(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *Handler) CategoryReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.repo.CategoryReport(r.Context(), r.URL.Query().Get("month"), r.URL.Query().Get("allocation_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *Handler) AllocationReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.repo.AllocationReport(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *Handler) ReviewReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.repo.ReviewReport(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *Handler) CreateIncome(w http.ResponseWriter, r *http.Request) {
	var req CreateIncomeRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	income, err := h.repo.CreateIncome(r.Context(), chi.URLParam(r, "month"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, income)
}

func (h *Handler) UpdateIncome(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	income, err := h.repo.UpdateIncome(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, income)
}

func (h *Handler) DeleteIncome(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteIncome(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) CreateAllocation(w http.ResponseWriter, r *http.Request) {
	var req CreateAllocationRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	allocation, err := h.repo.CreateAllocation(r.Context(), chi.URLParam(r, "month"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, allocation)
}

func (h *Handler) AllocationDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.repo.AllocationDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) UpdateAllocation(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	allocation, err := h.repo.UpdateAllocation(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, allocation)
}

func (h *Handler) DeleteAllocation(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteAllocation(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	transaction, err := h.repo.CreateTransaction(r.Context(), chi.URLParam(r, "month"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, transaction)
}

func (h *Handler) SplitTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionSplitRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	result, err := h.repo.SplitTransaction(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) SplitTransactionDetail(w http.ResponseWriter, r *http.Request) {
	result, err := h.repo.SplitTransactionDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	transaction, err := h.repo.UpdateTransaction(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, transaction)
}

func (h *Handler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteTransaction(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) CreateAllocationTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateAllocationTemplateRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	template, err := h.repo.CreateAllocationTemplate(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, template)
}

func (h *Handler) UpdateAllocationTemplate(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	template, err := h.repo.UpdateAllocationTemplate(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, template)
}

func (h *Handler) DeleteAllocationTemplate(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteAllocationTemplate(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) CreateIncomeTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateIncomeTemplateRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	template, err := h.repo.CreateIncomeTemplate(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, template)
}

func (h *Handler) UpdateIncomeTemplate(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	template, err := h.repo.UpdateIncomeTemplate(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, template)
}

func (h *Handler) DeleteIncomeTemplate(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteIncomeTemplate(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	category, err := h.repo.CreateCategory(r.Context(), req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, category)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodePatch(w, r)
	if !ok {
		return
	}
	category, err := h.repo.UpdateCategory(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, category)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteCategory(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func decodePatch(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	var patch map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, 1<<20, &patch); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return nil, false
	}
	return patch, true
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, shared.ErrNotFound) {
		httputil.NotFound(w, r)
		return
	}
	httputil.WriteError(w, http.StatusBadRequest, err.Error())
}
