package wallet

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"private-workspace/internal/shared"
)

const UnsortedCategoryID = "wallet-category-unsorted"

var validAllocationTypes = map[string]bool{
	"fixed":        true,
	"flexible":     true,
	"sinking_fund": true,
	"one_off":      true,
}

var validAdjustmentReasons = map[string]bool{
	"rounding":           true,
	"missed_transaction": true,
	"cash_variance":      true,
	"manual_correction":  true,
}

func validateMonth(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !shared.ValidateMonthKey(value) {
		return "", errors.New("month must use YYYY-MM")
	}
	return value, nil
}

func validateDate(value string, field string) (string, error) {
	value = strings.TrimSpace(value)
	if !shared.ValidateDateKey(value) {
		return "", fmt.Errorf("%s must use YYYY-MM-DD", field)
	}
	return value, nil
}

func normalizeAllocationType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	if !validAllocationTypes[value] {
		return "flexible"
	}
	return value
}

func normalizeAdjustmentReason(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	if value == "" {
		value = "manual_correction"
	}
	if !validAdjustmentReasons[value] {
		return "", errors.New("adjustment reason is invalid")
	}
	return value, nil
}

func normalizeRequiredName(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return value, nil
}

func normalizeOptionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func optionalStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parsePatchInt64(raw json.RawMessage, field string) (int64, error) {
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return value, nil
}

func parsePatchInt(raw json.RawMessage, field string) (int, error) {
	value, err := parsePatchInt64(raw, field)
	if err != nil {
		return 0, err
	}
	return int(value), nil
}

func parsePatchOptionalInt(raw json.RawMessage, field string) (*int, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	value, err := parsePatchInt(raw, field)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parsePatchBool(raw json.RawMessage, field string) (bool, error) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("%s must be a boolean", field)
	}
	return value, nil
}

func parsePatchOptionalString(raw json.RawMessage, field string) (*string, error) {
	value, err := shared.ParseOptionalString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be a string", field)
	}
	return optionalStringPtr(value), nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intBool(value int) bool {
	return value != 0
}

func ptrInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
