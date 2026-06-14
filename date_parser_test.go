package calendar

import (
	"testing"
	"time"
)

func TestParseNaturalDate_Keywords(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"today", true},
		{"tomorrow", true},
		{"yesterday", true},
		{"next monday", true},
		{"next friday", true},
		{"last tuesday", true},
		{"friday", true},
		{"gibberish xyz", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, ok := ParseNaturalDate(tt.input)
			if ok != tt.valid {
				t.Errorf("ParseNaturalDate(%q) valid=%v, want %v", tt.input, ok, tt.valid)
			}
		})
	}
}

func TestParseNaturalDate_Formats(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-04-23", "2026-04-23"},
		{"04/23/2026", "2026-04-23"},
		{"Jan 15, 2026", "2026-01-15"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed, ok := ParseNaturalDate(tt.input)
			if !ok {
				t.Fatalf("ParseNaturalDate(%q) returned false", tt.input)
			}
			got := parsed.Format("2006-01-02")
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseNaturalDate_Relative(t *testing.T) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)

	parsed, ok := ParseNaturalDate("3 days from now")
	if !ok {
		t.Fatal("expected valid parse for '3 days from now'")
	}
	expected := today.AddDate(0, 0, 3)
	if parsed != expected {
		t.Errorf("got %v, want %v", parsed, expected)
	}
}

func TestNewEventID(t *testing.T) {
	id := newEventID()
	if id == "" || len(id) != 32 {
		t.Errorf("expected 32 char hex id, got %d chars: %s", len(id), id)
	}
}
