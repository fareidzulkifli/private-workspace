package shared

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"private-workspace/internal/httputil"
)

const TimeFormat = time.RFC3339

var ErrNotFound = errors.New("not found")

type SQLer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewID() string {
	return uuid.NewString()
}

func NewToken(bytes int) (string, error) {
	if bytes < 32 {
		bytes = 32
	}
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func Now() string {
	return FormatTime(time.Now().UTC())
}

func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeFormat)
}

func NullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func StringPtr(value string) *string {
	return &value
}

func FromNullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func BoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func IntBool(value int) bool {
	return value != 0
}

func JSONTags(tags []string) string {
	b, err := json.Marshal(NormalizeTags(tags))
	if err != nil {
		return "[]"
	}
	return string(b)
}

func ParseTags(value string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return []string{}
	}
	return NormalizeTags(tags)
}

func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	out := make([]string, 0, min(len(tags), 20))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		out = append(out, tag)
		if len(out) == 20 {
			break
		}
	}
	return out
}

func WriteDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		httputil.NotFound(w, nil)
		return
	}
	httputil.WriteError(w, http.StatusInternalServerError, err.Error())
}

func ParseBoolQuery(r *http.Request, key string) bool {
	return r.URL.Query().Get(key) == "true"
}

func ParseOptionalFloat(raw json.RawMessage) (float64, error) {
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return n, nil
}

func ParseOptionalString(raw json.RawMessage) (*string, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func ParseRequiredString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func ParseOptionalBool(raw json.RawMessage) (bool, error) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func ParseOptionalTags(raw json.RawMessage) ([]string, error) {
	var tags []string
	if string(raw) == "null" {
		return []string{}, nil
	}
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil, err
	}
	return NormalizeTags(tags), nil
}

func QuerySingleResult(result sql.Result) (bool, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return true, nil
	}
	return affected > 0, nil
}

func ValidateDateKey(value string) bool {
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func ValidateMonthKey(value string) bool {
	if len(value) != 7 {
		return false
	}
	_, err := time.Parse("2006-01", value)
	return err == nil
}

func FloatFromQuery(r *http.Request, key string, fallback float64) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}
