package wallet

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"private-workspace/internal/shared"
)

func insertAllocationDefaultCategories(ctx context.Context, q shared.SQLer, allocationID string, categoryIDs []string, now string) error {
	for i, categoryID := range categoryIDs {
		if _, err := q.ExecContext(ctx, `INSERT INTO wallet_allocation_default_categories
			(allocation_id, category_id, sort_order, created_at)
			VALUES (?, ?, ?, ?)`,
			allocationID, categoryID, (i+1)*10, now); err != nil {
			return fmt.Errorf("save wallet allocation default category: %w", err)
		}
	}
	return nil
}

func categoryIDs(categories []Category) []string {
	ids := make([]string, 0, len(categories))
	for _, category := range categories {
		ids = append(ids, category.ID)
	}
	return ids
}

func (r *Repository) attachDefaultCategoriesToSummaries(ctx context.Context, summaries []AllocationSummary) ([]AllocationSummary, error) {
	if len(summaries) == 0 {
		return summaries, nil
	}
	allocations := make([]Allocation, len(summaries))
	for i := range summaries {
		allocations[i] = summaries[i].Allocation
	}
	allocations, err := r.attachDefaultCategoriesToAllocations(ctx, allocations)
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		summaries[i].Allocation = allocations[i]
	}
	return summaries, nil
}

func (r *Repository) attachDefaultCategoriesToAllocations(ctx context.Context, allocations []Allocation) ([]Allocation, error) {
	if len(allocations) == 0 {
		return allocations, nil
	}
	byID := make(map[string]int, len(allocations))
	allocationIDs := make([]string, 0, len(allocations))
	templateByAllocation := map[string]string{}
	for i := range allocations {
		allocations[i].DefaultCategories = []Category{}
		byID[allocations[i].ID] = i
		allocationIDs = append(allocationIDs, allocations[i].ID)
		if allocations[i].TemplateID != nil {
			templateByAllocation[allocations[i].ID] = *allocations[i].TemplateID
		}
	}

	hasCopiedDefaults := map[string]bool{}
	if err := loadAllocationDefaultCategoryRows(ctx, r.db, allocationIDs, func(allocationID string, category Category) {
		if index, ok := byID[allocationID]; ok {
			allocations[index].DefaultCategories = append(allocations[index].DefaultCategories, category)
			hasCopiedDefaults[allocationID] = true
		}
	}); err != nil {
		return nil, err
	}

	templateIDs := make([]string, 0)
	allocationTemplateIDs := map[string]string{}
	seenTemplates := map[string]bool{}
	for allocationID, templateID := range templateByAllocation {
		if hasCopiedDefaults[allocationID] {
			continue
		}
		allocationTemplateIDs[allocationID] = templateID
		if !seenTemplates[templateID] {
			seenTemplates[templateID] = true
			templateIDs = append(templateIDs, templateID)
		}
	}
	if len(templateIDs) == 0 {
		return allocations, nil
	}

	templateCategories, err := loadTemplateDefaultCategoryMap(ctx, r.db, templateIDs)
	if err != nil {
		return nil, err
	}
	for allocationID, templateID := range allocationTemplateIDs {
		index, ok := byID[allocationID]
		if !ok {
			continue
		}
		allocations[index].DefaultCategories = append(allocations[index].DefaultCategories, templateCategories[templateID]...)
	}
	return allocations, nil
}

func (r *Repository) defaultCategoriesForAllocation(ctx context.Context, allocation Allocation) ([]Category, error) {
	allocations, err := r.attachDefaultCategoriesToAllocations(ctx, []Allocation{allocation})
	if err != nil {
		return nil, err
	}
	return allocations[0].DefaultCategories, nil
}

func loadAllocationDefaultCategoryRows(ctx context.Context, q shared.SQLer, allocationIDs []string, apply func(string, Category)) error {
	if len(allocationIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(allocationIDs)), ",")
	args := make([]any, 0, len(allocationIDs))
	for _, id := range allocationIDs {
		args = append(args, id)
	}
	rows, err := q.QueryContext(ctx, `SELECT ac.allocation_id, c.id, c.name, c.system_key, c.active, c.sort_order, c.created_at, c.updated_at
		FROM wallet_allocation_default_categories ac
		JOIN wallet_categories c ON c.id = ac.category_id
		WHERE ac.allocation_id IN (`+placeholders+`)
		ORDER BY ac.sort_order ASC, c.sort_order ASC, c.name ASC`, args...)
	if err != nil {
		return fmt.Errorf("list wallet allocation default categories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var allocationID string
		category, err := scanCategoryWithPrefix(rows, &allocationID)
		if err != nil {
			return err
		}
		apply(allocationID, category)
	}
	return rows.Err()
}

func loadTemplateDefaultCategoryMap(ctx context.Context, q shared.SQLer, templateIDs []string) (map[string][]Category, error) {
	result := make(map[string][]Category, len(templateIDs))
	if len(templateIDs) == 0 {
		return result, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(templateIDs)), ",")
	args := make([]any, 0, len(templateIDs))
	for _, id := range templateIDs {
		args = append(args, id)
		result[id] = []Category{}
	}
	rows, err := q.QueryContext(ctx, `SELECT tc.template_id, c.id, c.name, c.system_key, c.active, c.sort_order, c.created_at, c.updated_at
		FROM wallet_allocation_template_categories tc
		JOIN wallet_categories c ON c.id = tc.category_id
		WHERE tc.template_id IN (`+placeholders+`)
		ORDER BY tc.sort_order ASC, c.sort_order ASC, c.name ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list wallet template default categories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var templateID string
		category, err := scanCategoryWithPrefix(rows, &templateID)
		if err != nil {
			return nil, err
		}
		result[templateID] = append(result[templateID], category)
	}
	return result, rows.Err()
}

func scanCategoryWithPrefix(scanner interface{ Scan(dest ...any) error }, prefix *string) (Category, error) {
	var category Category
	var systemKey sql.NullString
	var active int
	if err := scanner.Scan(prefix, &category.ID, &category.Name, &systemKey, &active, &category.SortOrder,
		&category.CreatedAt, &category.UpdatedAt); err != nil {
		return Category{}, err
	}
	category.SystemKey = shared.FromNullString(systemKey)
	category.Active = intBool(active)
	return category, nil
}
