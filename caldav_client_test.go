package calendar

import (
	"strings"
	"testing"
	"time"
)

func TestParseICS(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:test-uid-123@example.com
SUMMARY:Team Meeting
DESCRIPTION:Weekly standup
LOCATION:Conference Room B
DTSTART:20260427T090000Z
DTEND:20260427T100000Z
END:VEVENT
END:VCALENDAR`

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.UID != "test-uid-123@example.com" {
		t.Errorf("UID = %s", e.UID)
	}
	if e.Summary != "Team Meeting" {
		t.Errorf("Summary = %s", e.Summary)
	}
	if e.Location != "Conference Room B" {
		t.Errorf("Location = %s", e.Location)
	}
	if e.Start.Hour() != 9 {
		t.Errorf("Start hour = %d, want 9", e.Start.Hour())
	}
}

func TestParseICS_AllDay(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:allday-123@example.com
SUMMARY:Holiday
DTSTART;VALUE=DATE:20260501
DTEND;VALUE=DATE:20260502
END:VEVENT
END:VCALENDAR`

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].AllDay {
		t.Error("expected all-day event")
	}
}

func TestParseMultiStatus(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:response>
    <d:href>/cal/event1.ics</d:href>
    <d:propstat>
      <d:prop>
        <d:getetag>"abc123"</d:getetag>
        <c:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:event1@example.com
SUMMARY:Lunch
DTSTART:20260427T120000Z
DTEND:20260427T130000Z
END:VEVENT
END:VCALENDAR</c:calendar-data>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`

	events, err := parseMultiStatus(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("parseMultiStatus: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Summary != "Lunch" {
		t.Errorf("Summary = %s, want Lunch", events[0].Summary)
	}
}

func TestParseICS_MultipleEvents(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:uid1@example.com
SUMMARY:Event 1
DTSTART:20260427T090000Z
DTEND:20260427T100000Z
END:VEVENT
BEGIN:VEVENT
UID:uid2@example.com
SUMMARY:Event 2
DTSTART:20260427T140000Z
DTEND:20260427T150000Z
END:VEVENT
END:VCALENDAR`

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestCalDAVPresets(t *testing.T) {
	if len(CalDAVPresets) < 4 {
		t.Errorf("expected at least 4 presets, got %d", len(CalDAVPresets))
	}
	google := CalDAVPresets[0]
	if google.Name != "Google Calendar" {
		t.Errorf("first preset = %s, want Google Calendar", google.Name)
	}
}

func TestGenerateICS(t *testing.T) {
	start := time.Date(2026, 4, 28, 14, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC)
	ics := generateICS("test-uid-gen@localitas", "Dentist", "Checkup", "123 Main St", "", start, end, false)

	if !strings.Contains(ics, "UID:test-uid-gen@localitas") {
		t.Error("missing UID")
	}
	if !strings.Contains(ics, "SUMMARY:Dentist") {
		t.Error("missing SUMMARY")
	}
	if !strings.Contains(ics, "DESCRIPTION:Checkup") {
		t.Error("missing DESCRIPTION")
	}
	if !strings.Contains(ics, "LOCATION:123 Main St") {
		t.Error("missing LOCATION")
	}
	if !strings.Contains(ics, "DTSTART:20260428T140000Z") {
		t.Error("missing DTSTART")
	}
	if !strings.Contains(ics, "DTEND:20260428T150000Z") {
		t.Error("missing DTEND")
	}
	if !strings.Contains(ics, "BEGIN:VCALENDAR") || !strings.Contains(ics, "END:VCALENDAR") {
		t.Error("missing VCALENDAR wrapper")
	}
}

func TestGenerateICS_AllDay(t *testing.T) {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 23, 59, 59, 0, time.UTC)
	ics := generateICS("allday@localitas", "Holiday", "", "", "", start, end, true)

	if !strings.Contains(ics, "DTSTART;VALUE=DATE:20260501") {
		t.Errorf("expected DATE dtstart, got: %s", ics)
	}
	if !strings.Contains(ics, "DTEND;VALUE=DATE:20260502") {
		t.Errorf("expected DATE dtend, got: %s", ics)
	}
	if strings.Contains(ics, "DESCRIPTION:") {
		t.Error("should not include empty DESCRIPTION")
	}
	if strings.Contains(ics, "LOCATION:") {
		t.Error("should not include empty LOCATION")
	}
}

func TestGenerateICS_Roundtrip(t *testing.T) {
	start := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	end := time.Date(2026, 6, 15, 11, 30, 0, 0, time.UTC)
	ics := generateICS("roundtrip@localitas", "Team Sync", "Weekly meeting", "Room 42", "", start, end, false)

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS roundtrip: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.UID != "roundtrip@localitas" {
		t.Errorf("UID = %s", e.UID)
	}
	if e.Summary != "Team Sync" {
		t.Errorf("Summary = %s", e.Summary)
	}
	if e.Description != "Weekly meeting" {
		t.Errorf("Description = %s", e.Description)
	}
	if e.Location != "Room 42" {
		t.Errorf("Location = %s", e.Location)
	}
	if e.Start.Hour() != 10 || e.Start.Minute() != 30 {
		t.Errorf("Start = %v", e.Start)
	}
	if e.End.Hour() != 11 || e.End.Minute() != 30 {
		t.Errorf("End = %v", e.End)
	}
}

func TestNewEventUID(t *testing.T) {
	uid := newEventUID()
	if !strings.HasSuffix(uid, "@localitas") {
		t.Errorf("UID should end with @localitas, got %s", uid)
	}
	if len(uid) < 20 {
		t.Errorf("UID too short: %s", uid)
	}
	uid2 := newEventUID()
	if uid == uid2 {
		t.Error("two UIDs should be unique")
	}
}

func TestParseICS_Recurrence(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:recurring@example.com
SUMMARY:Weekly Standup
DTSTART:20260504T090000Z
DTEND:20260504T093000Z
RRULE:FREQ=WEEKLY;BYDAY=MO
END:VEVENT
END:VCALENDAR`

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Recurrence != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("Recurrence = %q, want FREQ=WEEKLY;BYDAY=MO", events[0].Recurrence)
	}
}

func TestExpandRecurringEvents_Weekly(t *testing.T) {
	e := &Event{
		ID:         "weekly-1",
		Title:      "Standup",
		StartTime:  time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2026, 5, 4, 9, 30, 0, 0, time.UTC),
		Recurrence: "FREQ=WEEKLY;BYDAY=MO",
	}

	rangeStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)

	expanded := expandRecurringEvents([]*Event{e}, rangeStart, rangeEnd)

	if len(expanded) < 4 {
		t.Errorf("expected at least 4 Monday occurrences in May, got %d", len(expanded))
	}
	for _, ev := range expanded {
		if ev.StartTime.Weekday() != time.Monday {
			t.Errorf("expected Monday, got %s for %v", ev.StartTime.Weekday(), ev.StartTime)
		}
		if ev.Title != "Standup" {
			t.Errorf("Title = %s", ev.Title)
		}
		duration := ev.EndTime.Sub(ev.StartTime)
		if duration != 30*time.Minute {
			t.Errorf("duration = %v, want 30m", duration)
		}
	}
}

func TestExpandRecurringEvents_NonRecurring(t *testing.T) {
	e := &Event{
		ID:        "single-1",
		Title:     "One-off",
		StartTime: time.Date(2026, 5, 10, 14, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 10, 15, 0, 0, 0, time.UTC),
	}

	rangeStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)

	expanded := expandRecurringEvents([]*Event{e}, rangeStart, rangeEnd)

	if len(expanded) != 1 {
		t.Errorf("expected 1 event, got %d", len(expanded))
	}
}

func TestExpandRecurringEvents_Daily(t *testing.T) {
	e := &Event{
		ID:         "daily-1",
		Title:      "Morning Review",
		StartTime:  time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2026, 5, 1, 8, 15, 0, 0, time.UTC),
		Recurrence: "FREQ=DAILY;COUNT=5",
	}

	rangeStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)

	expanded := expandRecurringEvents([]*Event{e}, rangeStart, rangeEnd)

	if len(expanded) != 5 {
		t.Errorf("expected 5 occurrences, got %d", len(expanded))
	}
}

func TestGenerateICS_WithTimezone(t *testing.T) {
	start := time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC)
	ics := generateICS("tz-test@localitas", "Meeting", "", "", "America/New_York", start, end, false)

	if !strings.Contains(ics, "DTSTART;TZID=America/New_York:20260501T140000") {
		t.Errorf("expected TZID DTSTART, got:\n%s", ics)
	}
	if !strings.Contains(ics, "DTEND;TZID=America/New_York:20260501T150000") {
		t.Errorf("expected TZID DTEND, got:\n%s", ics)
	}
	if strings.Contains(ics, "DTSTART:20260501T180000Z") {
		t.Error("should not have UTC DTSTART when timezone is set")
	}
}

func TestGenerateICS_WithoutTimezone(t *testing.T) {
	start := time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 19, 0, 0, 0, time.UTC)
	ics := generateICS("no-tz@localitas", "Meeting", "", "", "", start, end, false)

	if !strings.Contains(ics, "DTSTART:20260501T180000Z") {
		t.Errorf("expected UTC DTSTART, got:\n%s", ics)
	}
}

func TestParseICS_WithTimezone(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:tz-parse@example.com
SUMMARY:NYC Meeting
DTSTART;TZID=America/New_York:20260501T140000
DTEND;TZID=America/New_York:20260501T150000
END:VEVENT
END:VCALENDAR`

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want America/New_York", events[0].Timezone)
	}
	if events[0].Start.UTC().Hour() != 18 {
		t.Errorf("Start UTC hour = %d, want 18", events[0].Start.UTC().Hour())
	}
}

func TestGenerateICS_TimezoneRoundtrip(t *testing.T) {
	start := time.Date(2026, 6, 15, 22, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 15, 23, 0, 0, 0, time.UTC)
	ics := generateICS("rt-tz@localitas", "Late Call", "", "", "America/Los_Angeles", start, end, false)

	events, err := parseICS(ics)
	if err != nil {
		t.Fatalf("parseICS: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Timezone != "America/Los_Angeles" {
		t.Errorf("Timezone = %q, want America/Los_Angeles", e.Timezone)
	}
	if e.Start.UTC().Hour() != 22 {
		t.Errorf("Start UTC hour = %d, want 22", e.Start.UTC().Hour())
	}
}
