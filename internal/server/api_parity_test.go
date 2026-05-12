package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"private-workspace/internal/ai"
	"private-workspace/internal/auth"
	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/holidays"
	"private-workspace/internal/security"
	"private-workspace/internal/web"
)

func TestPrivateAPIGroupsRequireAuthAndCSRF(t *testing.T) {
	router, _, cleanup := newAPITestRouter(t, apiTestDeps{})
	defer cleanup()

	privateGETs := []string{
		"/api/nav",
		"/api/dashboard",
		"/api/orgs",
		"/api/projects",
		"/api/tasks",
		"/api/prompts/templates",
		"/api/gitnote/tree",
		"/api/holidays/malaysia",
		"/api/shares/gitnote",
		"/api/wallet/months",
		"/api/wallet/settings",
		"/api/wallet/months/2026-05/review",
		"/api/wallet/reports/monthly",
		"/api/wallet/reports/categories",
		"/api/wallet/reports/allocations",
		"/api/wallet/reports/review",
	}
	for _, path := range privateGETs {
		rec := perform(router, http.MethodGet, path, nil, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	rec := perform(router, http.MethodGet, "/api/share/gitnote/missing", nil, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public share read status = %d body=%s", rec.Code, rec.Body.String())
	}

	cookie, _ := login(t, router, "admin@example.com", "correct")
	unsafe := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/api/orgs", jsonBody(map[string]any{"name": "Ops"})},
		{http.MethodPost, "/api/projects", jsonBody(map[string]any{"name": "P", "org_id": "o"})},
		{http.MethodPost, "/api/tasks", jsonBody(map[string]any{"summary": "T", "project_id": "p"})},
		{http.MethodPost, "/api/dashboard/events", jsonBody(map[string]any{"title": "E", "event_date": "2026-05-10"})},
		{http.MethodPost, "/api/prompts/templates", jsonBody(map[string]any{"title": "P", "body": "B"})},
		{http.MethodPost, "/api/prompts/context-packs", jsonBody(map[string]any{"title": "C"})},
		{http.MethodPost, "/api/upload/presign", jsonBody(map[string]any{"filename": "a.txt", "contentType": "text/plain"})},
		{http.MethodPost, "/api/ai/task-summarize", jsonBody(map[string]any{"notes": "abc"})},
		{http.MethodPost, "/api/shares/gitnote", jsonBody(map[string]any{"pathPrefix": "notes/a.md"})},
		{http.MethodPost, "/api/wallet/months", jsonBody(map[string]any{"month": "2026-05"})},
		{http.MethodPost, "/api/wallet/months/2026-05/close", nil},
		{http.MethodPost, "/api/wallet/months/2026-05/reopen", nil},
		{http.MethodPost, "/api/wallet/months/2026-05/balance-updates", jsonBody(map[string]any{"new_balance_cents": 100})},
		{http.MethodPost, "/api/wallet/months/2026-05/reconciliation-adjustments", jsonBody(map[string]any{"amount_cents": 1, "reason": "rounding"})},
		{http.MethodPost, "/api/wallet/transactions/t/split", jsonBody(map[string]any{"splits": []any{}})},
		{http.MethodPost, "/api/wallet/allocation-templates", jsonBody(map[string]any{"name": "Work"})},
		{http.MethodPost, "/api/wallet/income-templates", jsonBody(map[string]any{"name": "Salary"})},
		{http.MethodPost, "/api/wallet/categories", jsonBody(map[string]any{"name": "Lunch"})},
	}
	for _, tc := range unsafe {
		rec := perform(router, tc.method, tc.path, tc.body, cookie, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestOrgProjectTaskNavArchiveAndDashboard(t *testing.T) {
	router, _, cleanup := newAPITestRouter(t, apiTestDeps{holiday: fakeHolidayClient{}})
	defer cleanup()
	cookie, csrf := login(t, router, "admin@example.com", "correct")

	org := apiJSON[map[string]any](t, router, http.MethodPost, "/api/orgs", jsonBody(map[string]any{"name": "Product Ops", "order_index": 1}), cookie, csrf, http.StatusCreated)
	if org["slug"] != "product-ops" {
		t.Fatalf("slug = %v", org["slug"])
	}
	rec := perform(router, http.MethodPost, "/api/orgs", jsonBody(map[string]any{"name": "Product Ops"}), cookie, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate org status = %d body=%s", rec.Code, rec.Body.String())
	}
	bySlug := apiJSON[map[string]any](t, router, http.MethodGet, "/api/orgs/by-slug/product-ops", nil, cookie, "", http.StatusOK)
	if bySlug["id"] != org["id"] {
		t.Fatalf("by-slug id = %v want %v", bySlug["id"], org["id"])
	}

	project := apiJSON[map[string]any](t, router, http.MethodPost, "/api/projects", jsonBody(map[string]any{"name": "Launch", "org_id": org["id"], "order_index": 2}), cookie, csrf, http.StatusCreated)
	task := apiJSON[map[string]any](t, router, http.MethodPost, "/api/tasks", jsonBody(map[string]any{
		"summary":    "Write launch checklist",
		"project_id": project["id"],
		"urgent":     true,
		"important":  true,
	}), cookie, csrf, http.StatusCreated)

	nav := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/nav", nil, cookie, "", http.StatusOK)
	projects := nav[0]["projects"].([]any)
	firstProject := projects[0].(map[string]any)
	if firstProject["incomplete_tasks_count"].(float64) != 1 {
		t.Fatalf("incomplete count = %#v", firstProject["incomplete_tasks_count"])
	}

	archiveAttempt := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/projects/"+project["id"].(string), jsonBody(map[string]any{"archived": true}), cookie, csrf, http.StatusBadRequest)
	if !strings.Contains(archiveAttempt["error"].(string), "all tasks are done") {
		t.Fatalf("archive error = %v", archiveAttempt["error"])
	}

	done := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/tasks/"+task["id"].(string), jsonBody(map[string]any{"status": "Done"}), cookie, csrf, http.StatusOK)
	if done["completed_at"] == nil {
		t.Fatal("completed_at was not set")
	}
	editedDone := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/tasks/"+task["id"].(string), jsonBody(map[string]any{"completed_at": "2026-05-09T15:30:00+08:00"}), cookie, csrf, http.StatusOK)
	if editedDone["completed_at"] != "2026-05-09T07:30:00Z" {
		t.Fatalf("edited completed_at = %#v", editedDone["completed_at"])
	}
	restored := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/tasks/"+task["id"].(string), jsonBody(map[string]any{"status": "In Progress"}), cookie, csrf, http.StatusOK)
	if restored["completed_at"] != nil {
		t.Fatalf("completed_at after restore = %#v", restored["completed_at"])
	}
	_, _ = apiJSON[map[string]any](t, router, http.MethodPatch, "/api/tasks/"+task["id"].(string), jsonBody(map[string]any{"status": "Done"}), cookie, csrf, http.StatusOK), done
	archived := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/projects/"+project["id"].(string), jsonBody(map[string]any{"archived": true}), cookie, csrf, http.StatusOK)
	if archived["archived_at"] == nil {
		t.Fatal("archived_at was not set")
	}
	restoredProject := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/projects/"+project["id"].(string), jsonBody(map[string]any{"archived": false}), cookie, csrf, http.StatusOK)
	if restoredProject["archived_at"] != nil {
		t.Fatalf("archived_at after restore = %#v", restoredProject["archived_at"])
	}

	dashboard := apiJSON[map[string]any](t, router, http.MethodGet, "/api/dashboard?month=2026-05", nil, cookie, "", http.StatusOK)
	if dashboard["calendar"].(map[string]any)["monthKey"] != "2026-05" {
		t.Fatalf("dashboard month = %#v", dashboard["calendar"])
	}
	if len(dashboard["kpis"].([]any)) == 0 {
		t.Fatal("dashboard kpis are empty")
	}
}

func TestWalletAPISummaryFlow(t *testing.T) {
	router, _, cleanup := newAPITestRouter(t, apiTestDeps{})
	defer cleanup()
	cookie, csrf := login(t, router, "admin@example.com", "correct")

	settings := apiJSON[map[string]any](t, router, http.MethodGet, "/api/wallet/settings", nil, cookie, "", http.StatusOK)
	if len(settings["allocation_templates"].([]any)) == 0 || len(settings["categories"].([]any)) == 0 {
		t.Fatalf("wallet settings = %#v", settings)
	}
	customTemplate := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/allocation-templates", jsonBody(map[string]any{
		"name":                 "Travel Fund",
		"default_amount_cents": 5000,
		"type":                 "sinking_fund",
		"carry_forward":        true,
	}), cookie, csrf, http.StatusCreated)
	patchedTemplate := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/wallet/allocation-templates/"+customTemplate["id"].(string), jsonBody(map[string]any{
		"default_amount_cents": 6000,
	}), cookie, csrf, http.StatusOK)
	if patchedTemplate["default_amount_cents"].(float64) != 6000 {
		t.Fatalf("patched template = %#v", patchedTemplate)
	}

	month := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months", jsonBody(map[string]any{
		"month":                 "2026-05",
		"opening_balance_cents": 100000,
		"use_templates":         true,
	}), cookie, csrf, http.StatusCreated)
	if month["month"] != "2026-05" {
		t.Fatalf("wallet month = %#v", month)
	}
	_ = apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/income", jsonBody(map[string]any{
		"name":             "Salary",
		"amount_cents":     500000,
		"received_date":    "2026-04-30",
		"applies_to_month": "2026-05",
	}), cookie, csrf, http.StatusCreated)
	allocation := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/allocations", jsonBody(map[string]any{
		"name":           "Work Expense",
		"budgeted_cents": 60000,
		"type":           "flexible",
	}), cookie, csrf, http.StatusCreated)
	transaction := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/transactions", jsonBody(map[string]any{
		"allocation_id": allocation["id"],
		"date":          "2026-05-10",
		"amount_cents":  2000,
		"rounded":       true,
	}), cookie, csrf, http.StatusCreated)
	if transaction["category_name"] != "Unsorted" {
		t.Fatalf("transaction category = %#v", transaction)
	}

	summary := apiJSON[map[string]any](t, router, http.MethodGet, "/api/wallet/months/2026-05/summary", nil, cookie, "", http.StatusOK)
	if summary["expected_balance_cents"].(float64) != 598000 || summary["variance_cents"].(float64) != 0 {
		t.Fatalf("wallet summary = %#v", summary)
	}
	if summary["total_reserved_cents"].(float64) != 64000 {
		t.Fatalf("reserved = %#v", summary["total_reserved_cents"])
	}
	review := summary["review_counts"].(map[string]any)
	if review["unsorted_count"].(float64) != 1 || review["rounded_count"].(float64) != 1 {
		t.Fatalf("review counts = %#v", review)
	}

	reviewList := apiJSON[map[string]any](t, router, http.MethodGet, "/api/wallet/months/2026-05/review", nil, cookie, "", http.StatusOK)
	if len(reviewList["transactions"].([]any)) != 1 {
		t.Fatalf("wallet review list = %#v", reviewList)
	}
	balanceUpdate := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/balance-updates", jsonBody(map[string]any{
		"new_balance_cents": 598100,
	}), cookie, csrf, http.StatusCreated)
	if balanceUpdate["adjustment"] != nil {
		t.Fatalf("balance update = %#v", balanceUpdate)
	}
	reconciliationTransaction := balanceUpdate["transaction"].(map[string]any)
	if reconciliationTransaction["kind"] != "income" || reconciliationTransaction["amount_cents"].(float64) != 100 {
		t.Fatalf("reconciliation transaction = %#v", reconciliationTransaction)
	}
	split := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/transactions/"+transaction["id"].(string)+"/split", jsonBody(map[string]any{
		"splits": []map[string]any{
			{"allocation_id": allocation["id"], "category_id": "wallet-category-lunch", "amount_cents": 800, "note": "Lunch"},
			{"allocation_id": allocation["id"], "category_id": "wallet-category-grab", "amount_cents": 1200, "note": "Grab"},
		},
	}), cookie, csrf, http.StatusCreated)
	if len(split["children"].([]any)) != 2 {
		t.Fatalf("split = %#v", split)
	}
	categoryReport := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/wallet/reports/categories?month=2026-05", nil, cookie, "", http.StatusOK)
	if len(categoryReport) != 2 {
		t.Fatalf("category report = %#v", categoryReport)
	}
	allocationReport := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/wallet/reports/allocations?month=2026-05", nil, cookie, "", http.StatusOK)
	if len(allocationReport) == 0 {
		t.Fatalf("allocation report = %#v", allocationReport)
	}
	monthlyReport := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/wallet/reports/monthly?from=2026-05&to=2026-05", nil, cookie, "", http.StatusOK)
	if len(monthlyReport) != 1 || monthlyReport[0]["variance_cents"].(float64) != 0 {
		t.Fatalf("monthly report = %#v", monthlyReport)
	}
	closed := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/close", nil, cookie, csrf, http.StatusOK)
	if closed["status"] != "closed" {
		t.Fatalf("closed month = %#v", closed)
	}
	reopened := apiJSON[map[string]any](t, router, http.MethodPost, "/api/wallet/months/2026-05/reopen", nil, cookie, csrf, http.StatusOK)
	if reopened["status"] != "open" {
		t.Fatalf("reopened month = %#v", reopened)
	}
}

func TestPromptsContextPacksAIUploadGitNoteAndShares(t *testing.T) {
	store := &fakeObjectStore{}
	git := &fakeGitNoteClient{
		tree: []gitnote.TreeItem{
			{Path: "notes/a.md", Size: 10},
			{Path: "notes/private/b.md", Size: 11},
			{Path: "other/c.md", Size: 12},
		},
		raw: map[string][]byte{
			"notes/a.md":         []byte("# A"),
			"notes/private/b.md": []byte("# B"),
		},
	}
	router, database, cleanup := newAPITestRouter(t, apiTestDeps{
		r2:      store,
		gitnote: git,
		ai:      fakeAIClient{response: `{"urgent":true,"important":false,"reasoning":"now"}`},
	})
	defer cleanup()
	cookie, csrf := login(t, router, "admin@example.com", "correct")

	tags := make([]string, 25)
	for i := range tags {
		tags[i] = "tag"
	}
	template := apiJSON[map[string]any](t, router, http.MethodPost, "/api/prompts/templates", jsonBody(map[string]any{
		"title": " Daily ",
		"body":  " Do work ",
		"tags":  tags,
	}), cookie, csrf, http.StatusCreated)
	if got := len(template["tags"].([]any)); got != 20 {
		t.Fatalf("tags length = %d", got)
	}
	archivedTemplate := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/prompts/templates/"+template["id"].(string), jsonBody(map[string]any{"archived": true}), cookie, csrf, http.StatusOK)
	if archivedTemplate["archived_at"] == nil {
		t.Fatal("prompt archived_at was not set")
	}

	pack := apiJSON[map[string]any](t, router, http.MethodPost, "/api/prompts/context-packs", jsonBody(map[string]any{
		"title": "Pack",
		"items": []map[string]any{{"title": "One", "body": "Body", "sort_order": 0}},
	}), cookie, csrf, http.StatusCreated)
	if got := len(pack["items"].([]any)); got != 1 {
		t.Fatalf("pack items = %d", got)
	}
	patchedPack := apiJSON[map[string]any](t, router, http.MethodPatch, "/api/prompts/context-packs/"+pack["id"].(string), jsonBody(map[string]any{
		"title": "Pack 2",
		"items": []map[string]any{{"title": "Two", "body": "Body 2", "sort_order": 0}},
	}), cookie, csrf, http.StatusOK)
	items := patchedPack["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Two" {
		t.Fatalf("patched items = %#v", items)
	}

	summary := apiJSON[map[string]any](t, router, http.MethodPost, "/api/ai/task-summarize", jsonBody(map[string]any{"notes": "Summarize me"}), cookie, csrf, http.StatusOK)
	if summary["summary"] != `{"urgent":true,"important":false,"reasoning":"now"}` {
		t.Fatalf("summary = %v", summary["summary"])
	}
	priority := apiJSON[map[string]any](t, router, http.MethodPost, "/api/ai/task-priority", jsonBody(map[string]any{"summary": "Ship"}), cookie, csrf, http.StatusOK)
	if priority["urgent"] != true || priority["important"] != false {
		t.Fatalf("priority = %#v", priority)
	}

	org := apiJSON[map[string]any](t, router, http.MethodPost, "/api/orgs", jsonBody(map[string]any{"name": "Files"}), cookie, csrf, http.StatusCreated)
	project := apiJSON[map[string]any](t, router, http.MethodPost, "/api/projects", jsonBody(map[string]any{"name": "Assets", "org_id": org["id"]}), cookie, csrf, http.StatusCreated)
	task := apiJSON[map[string]any](t, router, http.MethodPost, "/api/tasks", jsonBody(map[string]any{"summary": "Attach", "project_id": project["id"]}), cookie, csrf, http.StatusCreated)
	presign := apiJSON[map[string]any](t, router, http.MethodPost, "/api/upload/presign", jsonBody(map[string]any{"filename": "spec.md", "contentType": "text/markdown"}), cookie, csrf, http.StatusOK)
	if !strings.HasPrefix(presign["key"].(string), "uploads/") || presign["signedUrl"] == "" {
		t.Fatalf("presign = %#v", presign)
	}
	attachment := apiJSON[map[string]any](t, router, http.MethodPost, "/api/tasks/"+task["id"].(string)+"/attachments", jsonBody(map[string]any{
		"filename":   "spec.md",
		"r2_key":     presign["key"],
		"mime_type":  "text/markdown",
		"size_bytes": 42,
	}), cookie, csrf, http.StatusCreated)
	attachments := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/tasks/"+task["id"].(string)+"/attachments", nil, cookie, "", http.StatusOK)
	if len(attachments) != 1 || attachments[0]["url"] == "" {
		t.Fatalf("attachments = %#v", attachments)
	}
	_ = apiJSON[map[string]any](t, router, http.MethodDelete, "/api/tasks/"+task["id"].(string)+"/attachments?attachment_id="+attachment["id"].(string), nil, cookie, csrf, http.StatusOK)
	if store.deleted[0] != attachment["r2_key"] {
		t.Fatalf("deleted keys = %#v", store.deleted)
	}

	tree := apiJSON[map[string]any](t, router, http.MethodGet, "/api/gitnote/tree", nil, cookie, "", http.StatusOK)
	if len(tree["tree"].([]any)) != 3 {
		t.Fatalf("gitnote tree = %#v", tree)
	}
	rec := perform(router, http.MethodGet, "/api/gitnote/raw?path=..%2Fsecret.md", nil, cookie, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = perform(router, http.MethodGet, "/api/gitnote/raw?path=notes/a.md", nil, cookie, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "# A" {
		t.Fatalf("raw status=%d body=%s", rec.Code, rec.Body.String())
	}

	createdShare := apiJSON[map[string]any](t, router, http.MethodPost, "/api/shares/gitnote", jsonBody(map[string]any{
		"pathPrefix": "notes/private",
		"title":      "Private Notes",
		"expiresAt":  nil,
	}), cookie, csrf, http.StatusCreated)
	token := createdShare["token"].(string)
	if createdShare["url"] != "/share/"+token {
		t.Fatalf("share = %#v", createdShare)
	}
	if !strings.HasPrefix(token, "private-notes-") {
		t.Fatalf("share token should be readable, got %q", token)
	}
	suffix := strings.TrimPrefix(token, "private-notes-")
	if len(suffix) != 16 {
		t.Fatalf("share token suffix length = %d token=%q", len(suffix), token)
	}
	for _, r := range suffix {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			t.Fatalf("share token suffix contains unsafe character %q in %q", r, token)
		}
	}
	var tokenHash string
	if err := database.SQL().QueryRow("SELECT token_hash FROM gitnote_shares WHERE id = ?", createdShare["id"]).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tokenHash, token) || tokenHash == "" {
		t.Fatalf("token hash stored incorrectly: %q", tokenHash)
	}
	shares := apiJSON[[]map[string]any](t, router, http.MethodGet, "/api/shares/gitnote", nil, cookie, "", http.StatusOK)
	if len(shares) != 1 {
		t.Fatalf("shares = %#v", shares)
	}
	if _, ok := shares[0]["token"]; ok {
		t.Fatalf("list leaked token: %#v", shares[0])
	}
	publicTree := apiJSON[map[string]any](t, router, http.MethodGet, "/api/share/gitnote/"+token+"/tree", nil, nil, "", http.StatusOK)
	if got := len(publicTree["tree"].([]any)); got != 1 {
		t.Fatalf("public tree count = %d body=%#v", got, publicTree)
	}
	rec = perform(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path=notes/private/b.md", nil, nil, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "# B" {
		t.Fatalf("public raw status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = perform(router, http.MethodGet, "/api/share/gitnote/"+token+"/raw?path=notes/a.md", nil, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("outside prefix status=%d body=%s", rec.Code, rec.Body.String())
	}
	_ = apiJSON[map[string]any](t, router, http.MethodDelete, "/api/shares/gitnote/"+createdShare["id"].(string), nil, cookie, csrf, http.StatusOK)
	rec = perform(router, http.MethodGet, "/api/share/gitnote/"+token, nil, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoked public status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAIPriorityMalformedJSONReturnsError(t *testing.T) {
	router, _, cleanup := newAPITestRouter(t, apiTestDeps{ai: fakeAIClient{response: "not-json"}})
	defer cleanup()
	cookie, csrf := login(t, router, "admin@example.com", "correct")
	rec := perform(router, http.MethodPost, "/api/ai/task-priority", jsonBody(map[string]any{"summary": "Ship"}), cookie, csrf)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid format") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

type apiTestDeps struct {
	r2      *fakeObjectStore
	gitnote gitnote.Client
	holiday holidays.Client
	ai      ai.Client
}

func newAPITestRouter(t *testing.T, deps apiTestDeps) (http.Handler, *db.DB, func()) {
	t.Helper()
	emptyDir := t.TempDir()
	database, err := db.Open(context.Background(), db.Config{
		Path:          filepath.Join(t.TempDir(), "private-workspace.db"),
		MigrationsDir: testMigrationsDir(t),
		AppEnv:        "development",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := auth.NewStore(database, time.Hour)
	if _, err := store.BootstrapAdmin(context.Background(), "admin@example.com", testAdminHash(t)); err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(store, auth.Options{
		CookieName:     "pw_session",
		CookieSecure:   false,
		CSRFHeaderName: "X-CSRF-Token",
		Limiter:        security.NewLoginLimiter(5, 15*time.Minute),
	})
	if deps.holiday == nil {
		deps.holiday = fakeHolidayClient{}
	}
	if deps.ai == nil {
		deps.ai = fakeAIClient{response: "ok"}
	}
	router := NewRouter(Config{
		DB:           database,
		Auth:         authService,
		Web:          web.New(web.Options{DistDir: filepath.Join(emptyDir, "dist"), PublicDir: filepath.Join(emptyDir, "public"), Favicon: filepath.Join(emptyDir, "favicon.ico")}),
		R2:           deps.r2,
		GitNote:      deps.gitnote,
		Holidays:     deps.holiday,
		HolidayState: "kuala-lumpur",
		AI:           deps.ai,
	})
	return router, database, func() { _ = database.Close() }
}

func jsonBody(value any) []byte {
	body, _ := json.Marshal(value)
	return body
}

func apiJSON[T any](t *testing.T, router http.Handler, method string, path string, body []byte, cookie *http.Cookie, csrf string, status int) T {
	t.Helper()
	rec := perform(router, method, path, body, cookie, csrf)
	if rec.Code != status {
		t.Fatalf("%s %s status = %d want %d body=%s", method, path, rec.Code, status, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v body=%s", path, err, rec.Body.String())
	}
	return out
}

type fakeObjectStore struct {
	deleted []string
}

func (f *fakeObjectStore) PresignPutObject(ctx context.Context, key string, contentType string, expires time.Duration) (string, error) {
	return "https://r2.example/upload/" + key, nil
}

func (f *fakeObjectStore) PresignGetObject(ctx context.Context, key string, expires time.Duration) (string, error) {
	return "https://r2.example/get/" + key, nil
}

func (f *fakeObjectStore) DeleteObject(ctx context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

type fakeGitNoteClient struct {
	tree []gitnote.TreeItem
	raw  map[string][]byte
}

func (f *fakeGitNoteClient) Tree(ctx context.Context) ([]gitnote.TreeItem, error) {
	return f.tree, nil
}

func (f *fakeGitNoteClient) Raw(ctx context.Context, filePath string) (gitnote.RawFile, error) {
	normalized, err := gitnote.NormalizePath(filePath)
	if err != nil {
		return gitnote.RawFile{}, err
	}
	body, ok := f.raw[normalized]
	if !ok {
		return gitnote.RawFile{}, gitnote.HTTPError{Status: http.StatusNotFound, Message: "not found"}
	}
	return gitnote.RawFile{ContentType: gitnote.ContentType(normalized), Body: body}, nil
}

type fakeHolidayClient struct{}

func (fakeHolidayClient) FetchMalaysiaHolidays(ctx context.Context, state string, year int) holidays.Response {
	return holidays.Response{
		State: "kuala-lumpur",
		Year:  year,
		Holidays: []holidays.Holiday{{
			ID:          "my-kuala-lumpur-2026-05-01-labour-day",
			Date:        "2026-05-01",
			Title:       "Labour Day",
			State:       "kuala-lumpur",
			DayOfWeek:   "Friday",
			IsMandatory: true,
			Source:      "test",
		}},
	}
}

type fakeAIClient struct {
	response string
}

func (f fakeAIClient) Query(ctx context.Context, prompt string, contextMessages []ai.Message) (string, error) {
	return f.response, nil
}
