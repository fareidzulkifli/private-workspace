package holidays

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/httputil"
)

const DefaultState = "kuala-lumpur"
const apiBase = "https://sabah-holiday.dydxsoft.my/api"

var stateRE = regexp.MustCompile(`^[a-z-]+$`)

var months = map[string]string{
	"Jan": "01",
	"Feb": "02",
	"Mar": "03",
	"Apr": "04",
	"May": "05",
	"Jun": "06",
	"Jul": "07",
	"Aug": "08",
	"Sep": "09",
	"Oct": "10",
	"Nov": "11",
	"Dec": "12",
}

type Holiday struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Title       string `json:"title"`
	State       string `json:"state"`
	DayOfWeek   string `json:"dayOfWeek"`
	IsMandatory bool   `json:"isMandatory"`
	Source      string `json:"source"`
}

type Response struct {
	State    string    `json:"state"`
	Year     int       `json:"year"`
	Holidays []Holiday `json:"holidays"`
}

type Client interface {
	FetchMalaysiaHolidays(ctx context.Context, state string, year int) Response
}

type HTTPClient struct {
	client       *http.Client
	defaultState string
}

func NewHTTPClient(defaultState string) *HTTPClient {
	defaultState = normalizeState(defaultState)
	return &HTTPClient{
		client:       &http.Client{Timeout: 10 * time.Second},
		defaultState: defaultState,
	}
}

func (c *HTTPClient) FetchMalaysiaHolidays(ctx context.Context, state string, year int) Response {
	if c == nil {
		return Response{State: normalizeState(state), Year: normalizeYear(year), Holidays: []Holiday{}}
	}
	state = normalizeStateWithDefault(state, c.defaultState)
	year = normalizeYear(year)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%d.json", apiBase, state, year), nil)
	if err != nil {
		return Response{State: state, Year: year, Holidays: []Holiday{}}
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return Response{State: state, Year: year, Holidays: []Holiday{}}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Response{State: state, Year: year, Holidays: []Holiday{}}
	}
	var upstream []struct {
		Date        string `json:"date"`
		HolidayName string `json:"holiday_name"`
		DayOfWeek   string `json:"day_of_week"`
		IsMandatory bool   `json:"is_mandatory"`
	}
	if err := json.NewDecoder(res.Body).Decode(&upstream); err != nil {
		return Response{State: state, Year: year, Holidays: []Holiday{}}
	}
	holidays := make([]Holiday, 0, len(upstream))
	for _, item := range upstream {
		if holiday, ok := normalizeHoliday(item.Date, item.HolidayName, item.DayOfWeek, item.IsMandatory, state, year); ok {
			holidays = append(holidays, holiday)
		}
	}
	return Response{State: state, Year: year, Holidays: holidays}
}

type Handler struct {
	client       Client
	defaultState string
}

func NewHandler(client Client, defaultState string) *Handler {
	if defaultState == "" {
		defaultState = DefaultState
	}
	return &Handler{client: client, defaultState: defaultState}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/holidays/malaysia", h.Malaysia)
}

func (h *Handler) Malaysia(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	year := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("year")); raw != "" {
		year, _ = strconv.Atoi(raw)
	}
	if h.client == nil {
		h.client = NewHTTPClient(h.defaultState)
	}
	resp := h.client.FetchMalaysiaHolidays(r.Context(), state, year)
	w.Header().Set("Cache-Control", "s-maxage=86400, stale-while-revalidate=604800")
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func normalizeState(value string) string {
	return normalizeStateWithDefault(value, DefaultState)
}

func normalizeStateWithDefault(value string, fallback string) string {
	candidate := strings.ToLower(strings.TrimSpace(value))
	if candidate == "" {
		candidate = fallback
	}
	if !stateRE.MatchString(candidate) {
		return DefaultState
	}
	return candidate
}

func normalizeYear(year int) int {
	currentYear := time.Now().Year()
	if year < 2020 || year > 2050 {
		return currentYear
	}
	return year
}

func normalizeHoliday(dateText string, title string, dayOfWeek string, mandatory bool, state string, year int) (Holiday, bool) {
	parts := strings.Fields(strings.TrimSpace(dateText))
	if len(parts) != 2 {
		return Holiday{}, false
	}
	month, ok := months[parts[0]]
	if !ok {
		return Holiday{}, false
	}
	day, err := strconv.Atoi(parts[1])
	if err != nil || day < 1 || day > 31 {
		return Holiday{}, false
	}
	if strings.TrimSpace(title) == "" {
		title = "Public Holiday"
	}
	date := fmt.Sprintf("%d-%s-%02d", year, month, day)
	return Holiday{
		ID:          fmt.Sprintf("my-%s-%s-%s", state, date, slugify(title)),
		Date:        date,
		Title:       title,
		State:       state,
		DayOfWeek:   dayOfWeek,
		IsMandatory: mandatory,
		Source:      "Malaysia Public Holidays API",
	}, true
}

func slugify(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
