package dashboard

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildCalendarReturnsEmptyArraysForDayLists(t *testing.T) {
	calendar := buildCalendar(nil, time.Date(2026, time.May, 10, 0, 0, 0, 0, time.Local), time.Date(2026, time.May, 1, 0, 0, 0, 0, time.Local), nil)
	body, err := json.Marshal(calendar)
	if err != nil {
		t.Fatalf("marshal calendar: %v", err)
	}

	var payload struct {
		Weeks [][]struct {
			Holidays       []any `json:"holidays"`
			CompletedTasks []any `json:"completedTasks"`
		} `json:"weeks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal calendar: %v", err)
	}

	for weekIndex, week := range payload.Weeks {
		for dayIndex, day := range week {
			if day.Holidays == nil {
				t.Fatalf("weeks[%d][%d].holidays encoded as null", weekIndex, dayIndex)
			}
			if day.CompletedTasks == nil {
				t.Fatalf("weeks[%d][%d].completedTasks encoded as null", weekIndex, dayIndex)
			}
		}
	}
}
