package calendar

import (
	"strings"
	"testing"
	"time"

	ical "github.com/emersion/go-ical"
)

func TestExtractCalendarID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/caldav/abc123/", "abc123"},
		{"/caldav/abc123/event.ics", "abc123"},
		{"/caldav/", ""},
		{"/caldav/abc123", "abc123"},
	}
	for _, tt := range tests {
		got := extractCalendarID(tt.path)
		if got != tt.want {
			t.Errorf("extractCalendarID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractEventUID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/caldav/abc123/myevent@localitas.ics", "myevent@localitas"},
		{"/caldav/abc123/simple.ics", "simple"},
		{"/caldav/abc123/", ""},
		{"/caldav/", ""},
	}
	for _, tt := range tests {
		got := extractEventUID(tt.path)
		if got != tt.want {
			t.Errorf("extractEventUID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestEventToICalendar(t *testing.T) {
	e := &Event{
		ID:          "test-id",
		RemoteUID:   "test-uid@localitas",
		Title:       "Dentist",
		Description: "Checkup",
		Location:    "123 Main St",
		StartTime:   time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC),
		AllDay:      false,
		UpdatedAt:   time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
	}

	cal := eventToICalendar(e)

	if cal == nil {
		t.Fatal("expected non-nil calendar")
	}
	if len(cal.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(cal.Children))
	}

	vevent := cal.Children[0]
	if vevent.Name != ical.CompEvent {
		t.Errorf("expected VEVENT, got %s", vevent.Name)
	}

	event := ical.Event{Component: vevent}
	uid, _ := event.Props.Text(ical.PropUID)
	if uid != "test-uid@localitas" {
		t.Errorf("UID = %s", uid)
	}
	summary, _ := event.Props.Text(ical.PropSummary)
	if summary != "Dentist" {
		t.Errorf("Summary = %s", summary)
	}
	desc, _ := event.Props.Text(ical.PropDescription)
	if desc != "Checkup" {
		t.Errorf("Description = %s", desc)
	}
	loc, _ := event.Props.Text(ical.PropLocation)
	if loc != "123 Main St" {
		t.Errorf("Location = %s", loc)
	}
}

func TestEventToICalendar_AllDay(t *testing.T) {
	e := &Event{
		ID:        "allday-id",
		RemoteUID: "allday@localitas",
		Title:     "Holiday",
		StartTime: time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 7, 4, 23, 59, 59, 0, time.UTC),
		AllDay:    true,
		UpdatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}

	cal := eventToICalendar(e)
	vevent := cal.Children[0]
	event := ical.Event{Component: vevent}

	startProp := event.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		t.Fatal("missing DTSTART")
	}
	if v := startProp.Params.Get("VALUE"); v != "DATE" {
		t.Errorf("DTSTART VALUE = %s, want DATE", v)
	}
}

func TestEventToCalendarObject(t *testing.T) {
	e := &Event{
		ID:        "obj-id",
		RemoteUID: "obj-uid@localitas",
		Title:     "Meeting",
		StartTime: time.Date(2026, 5, 10, 14, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 10, 15, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
	}

	obj := eventToCalendarObject(e, "/caldav/cal1/")

	if !strings.HasSuffix(obj.Path, "obj-uid@localitas.ics") {
		t.Errorf("Path = %s", obj.Path)
	}
	if obj.ETag == "" {
		t.Error("expected non-empty ETag")
	}
	if obj.Data == nil {
		t.Error("expected non-nil Data")
	}
}

func TestEventToICalendar_FallbackUID(t *testing.T) {
	e := &Event{
		ID:        "local-only",
		Title:     "Local Event",
		StartTime: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
	}

	cal := eventToICalendar(e)
	vevent := cal.Children[0]
	event := ical.Event{Component: vevent}
	uid, _ := event.Props.Text(ical.PropUID)
	if uid != "local-only@localitas" {
		t.Errorf("UID = %s, want local-only@localitas", uid)
	}
}
