package phase1

import (
	"testing"
	"time"
)

func TestNormalizeValueTimestampUTC(t *testing.T) {
	input := time.Date(2026, 5, 10, 12, 30, 0, 0, time.FixedZone("MYT", 8*60*60))
	got, err := NormalizeValue(input, KindTimestamp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-05-10T04:30:00Z" {
		t.Fatalf("expected UTC timestamp, got %v", got)
	}
}

func TestNormalizeValueUUIDBytes(t *testing.T) {
	input := [16]byte{41, 76, 249, 135, 35, 11, 72, 85, 174, 83, 110, 215, 186, 126, 220, 170}
	got, err := NormalizeValue(input, KindText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "294cf987-230b-4855-ae53-6ed7ba7edcaa" {
		t.Fatalf("expected canonical UUID, got %v", got)
	}
}

func TestNormalizeValueDate(t *testing.T) {
	got, err := NormalizeValue("2026-05-10T04:30:00Z", KindDate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-05-10" {
		t.Fatalf("expected date-only string, got %v", got)
	}
}

func TestNormalizeValueBool(t *testing.T) {
	got, err := NormalizeValue(true, KindBool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestNormalizeValueJSONCompacts(t *testing.T) {
	got, err := NormalizeValue(`["work", "focus"]`, KindJSON, "[]")
	if err != nil {
		t.Fatal(err)
	}
	if got != `["work","focus"]` {
		t.Fatalf("expected compact JSON, got %v", got)
	}
}

func TestNormalizeValueJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := NormalizeValue(`not-json`, KindJSON, "[]"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
