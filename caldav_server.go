package calendar

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

const caldavPrefix = "/caldav/"

type CalDAVBackend struct {
	store *Store
}

func NewCalDAVBackend(store *Store) *CalDAVBackend {
	return &CalDAVBackend{store: store}
}

func NewCalDAVHandler(store *Store, prefix string) *caldav.Handler {
	return &caldav.Handler{
		Backend: NewCalDAVBackend(store),
		Prefix:  prefix,
	}
}

func (b *CalDAVBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/caldav/", nil
}

func (b *CalDAVBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return "/caldav/", nil
}

func (b *CalDAVBackend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	_, err := b.store.UpsertCalendar(ctx, "", calendar.Name, calendar.Path, "#7d6b96")
	return err
}

func (b *CalDAVBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	cals, err := b.store.ListAllCalendars(ctx)
	if err != nil {
		return nil, err
	}
	if len(cals) == 0 {
		cal, err := b.store.UpsertCalendar(ctx, "", "Default", "/caldav/default/", "#7d6b96")
		if err != nil {
			return nil, err
		}
		cals = []*Calendar{cal}
	}
	out := make([]caldav.Calendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, caldav.Calendar{
			Path:                  "/caldav/" + c.ID + "/",
			Name:                  c.Name,
			Description:           c.Name,
			SupportedComponentSet: []string{"VEVENT"},
		})
	}
	return out, nil
}

func (b *CalDAVBackend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	calID := extractCalendarID(path)
	if calID == "" {
		return nil, fmt.Errorf("calendar not found")
	}
	c, err := b.store.GetCalendar(ctx, calID)
	if err != nil {
		return nil, err
	}
	return &caldav.Calendar{
		Path:                  "/caldav/" + c.ID + "/",
		Name:                  c.Name,
		Description:           c.Name,
		SupportedComponentSet: []string{"VEVENT"},
	}, nil
}

func (b *CalDAVBackend) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	uid := extractEventUID(path)
	if uid == "" {
		return nil, fmt.Errorf("event not found")
	}
	e, err := b.store.GetEventByRemoteUID(ctx, uid)
	if err != nil {
		return nil, err
	}
	return eventToCalendarObject(e, path), nil
}

func (b *CalDAVBackend) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	calID := extractCalendarID(path)
	if calID == "" {
		return nil, fmt.Errorf("calendar not found")
	}
	events, err := b.store.GetEventsByCalendarID(ctx, calID)
	if err != nil {
		return nil, err
	}
	return eventsToCalendarObjects(events, path), nil
}

func (b *CalDAVBackend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	calID := extractCalendarID(path)
	if calID == "" {
		return nil, fmt.Errorf("calendar not found")
	}

	var events []*Event
	var err error

	if !query.CompFilter.Start.IsZero() && !query.CompFilter.End.IsZero() {
		events, err = b.store.GetEventsByCalendarIDRange(ctx, calID, query.CompFilter.Start, query.CompFilter.End)
	} else {
		events, err = b.store.GetEventsByCalendarID(ctx, calID)
	}
	if err != nil {
		return nil, err
	}
	return eventsToCalendarObjects(events, path), nil
}

func (b *CalDAVBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	calID := extractCalendarID(path)
	if calID == "" {
		return nil, fmt.Errorf("calendar not found")
	}

	for _, comp := range calendar.Children {
		if comp.Name != ical.CompEvent {
			continue
		}
		event := ical.Event{Component: comp}

		uid, _ := event.Props.Text(ical.PropUID)
		summary, _ := event.Props.Text(ical.PropSummary)
		description, _ := event.Props.Text(ical.PropDescription)
		location, _ := event.Props.Text(ical.PropLocation)

		dtStart, err := event.DateTimeStart(time.UTC)
		if err != nil {
			continue
		}
		dtEnd, err := event.DateTimeEnd(time.UTC)
		if err != nil {
			dtEnd = dtStart.Add(time.Hour)
		}

		allDay := false
		var tz string
		if startProp := event.Props.Get(ical.PropDateTimeStart); startProp != nil {
			if v := startProp.Params.Get("VALUE"); v == "DATE" {
				allDay = true
			}
			if tzid := startProp.Params.Get("TZID"); tzid != "" {
				tz = tzid
			}
		}

		if uid == "" {
			uid = newEventUID()
		}

		existing, _ := b.store.GetEventByRemoteUID(ctx, uid)
		if existing != nil {
			b.store.Update(ctx, existing.ID, summary, description, location, dtStart, dtEnd, allDay)
		} else {
			b.store.Create(ctx, calID, "", uid, summary, description, location, tz, dtStart, dtEnd, allDay)
		}

		e, _ := b.store.GetEventByRemoteUID(ctx, uid)
		if e != nil {
			return eventToCalendarObject(e, path), nil
		}
	}

	return nil, fmt.Errorf("no event found in calendar data")
}

func (b *CalDAVBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	uid := extractEventUID(path)
	if uid == "" {
		return fmt.Errorf("event not found")
	}
	return b.store.DeleteEventByRemoteUID(ctx, uid)
}

func (b *CalDAVBackend) ListCalendarsJSON(ctx context.Context) ([]*Calendar, error) {
	return b.store.ListAllCalendars(ctx)
}

func (b *CalDAVBackend) GetEventsByDay(ctx context.Context, start, end time.Time) ([]DayEvents, error) {
	return b.store.GetEventsByDay(ctx, start, end)
}

func (b *CalDAVBackend) GetEvent(ctx context.Context, id string) (*Event, error) {
	return b.store.Get(ctx, id)
}

func (b *CalDAVBackend) CreateEventJSON(ctx context.Context, calendarID, title, description, location, timezone string, start, end time.Time, allDay bool) (*Event, error) {
	uid := newEventUID()

	cal := buildICalEvent(uid, title, description, location, timezone, start, end, allDay)

	if calendarID != "" {
		path := "/caldav/" + calendarID + "/"
		_, err := b.PutCalendarObject(ctx, path+uid+".ics", cal, nil)
		if err != nil {
			return nil, fmt.Errorf("caldav put: %w", err)
		}
		e, err := b.store.GetEventByRemoteUID(ctx, uid)
		if err != nil {
			return nil, err
		}

		storedCal, _ := b.store.GetCalendar(ctx, calendarID)
		if storedCal != nil {
			account, _ := b.store.GetAccount(ctx, storedCal.AccountID)
			if account != nil {
				syncEventToRemote(ctx, b.store, account, storedCal, uid, title, description, location, start, end, allDay)
			}
		}
		return e, nil
	}

	path := "/caldav/local/"
	_, err := b.PutCalendarObject(ctx, path+uid+".ics", cal, nil)
	if err != nil {
		return nil, fmt.Errorf("caldav put: %w", err)
	}
	return b.store.GetEventByRemoteUID(ctx, uid)
}

func (b *CalDAVBackend) UpdateEventJSON(ctx context.Context, id, title, description, location string, start, end time.Time, allDay bool) (*Event, error) {
	existing, err := b.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	uid := existing.RemoteUID
	if uid == "" {
		uid = existing.ID + "@localitas"
	}

	cal := buildICalEvent(uid, title, description, location, existing.Timezone, start, end, allDay)
	calID := existing.CalendarID
	if calID == "" {
		calID = "local"
	}
	path := "/caldav/" + calID + "/" + uid + ".ics"
	b.PutCalendarObject(ctx, path, cal, nil)

	if existing.RemoteUID != "" && existing.CalendarID != "" {
		storedCal, _ := b.store.GetCalendar(ctx, existing.CalendarID)
		if storedCal != nil {
			account, _ := b.store.GetAccount(ctx, storedCal.AccountID)
			if account != nil {
				syncEventToRemote(ctx, b.store, account, storedCal, existing.RemoteUID, title, description, location, start, end, allDay)
			}
		}
	}

	return b.store.Get(ctx, id)
}

func (b *CalDAVBackend) DeleteEventJSON(ctx context.Context, id string) error {
	existing, _ := b.store.Get(ctx, id)
	if existing == nil {
		return b.store.Delete(ctx, id)
	}

	if existing.RemoteUID != "" && existing.CalendarID != "" {
		caldavClient, err := getCalDAVClientForEvent(ctx, b.store, existing)
		if err == nil && caldavClient != nil {
			caldavClient.DeleteEvent(ctx, existing.RemoteUID)
		}

		uid := existing.RemoteUID
		calID := existing.CalendarID
		path := "/caldav/" + calID + "/" + uid + ".ics"
		b.DeleteCalendarObject(ctx, path)
		return nil
	}

	return b.store.Delete(ctx, id)
}

func (b *CalDAVBackend) SearchEvents(ctx context.Context, query string, limit int) ([]*Event, error) {
	return b.store.Search(ctx, query, limit)
}

func buildICalEvent(uid, title, description, location, timezone string, start, end time.Time, allDay bool) *ical.Calendar {
	e := &Event{
		RemoteUID:   uid,
		Title:       title,
		Description: description,
		Location:    location,
		Timezone:    timezone,
		StartTime:   start,
		EndTime:     end,
		AllDay:      allDay,
		UpdatedAt:   time.Now().UTC(),
	}
	return eventToICalendar(e)
}

func syncEventToRemote(ctx context.Context, store *Store, account *CalendarAccount, cal *Calendar, uid, title, description, location string, start, end time.Time, allDay bool) {
	if account.NeedsOAuth() {
		fullAcct, err := store.GetAccountWithTokens(ctx, account.ID)
		if err != nil {
			return
		}
		if err := EnsureValidToken(ctx, store, fullAcct); err != nil {
			return
		}
		caldavClient := NewCalDAVClientWithBearer(cal.Href, fullAcct.AccessToken)
		caldavClient.UpdateEvent(ctx, uid, title, description, location, start, end, allDay)
	} else if account.CalDAVURL != "" {
		caldavClient := NewCalDAVClient(account.CalDAVURL, account.Username, account.Password)
		caldavClient.UpdateEvent(ctx, uid, title, description, location, start, end, allDay)
	}
}

func extractCalendarID(path string) string {
	path = strings.TrimPrefix(path, "/caldav/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

func extractEventUID(path string) string {
	path = strings.TrimPrefix(path, "/caldav/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		name := parts[len(parts)-1]
		return strings.TrimSuffix(name, ".ics")
	}
	return ""
}

func eventToICalendar(e *Event) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//Localitas//Calendar//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")

	vevent := ical.NewComponent(ical.CompEvent)
	uid := e.RemoteUID
	if uid == "" {
		uid = e.ID + "@localitas"
	}
	vevent.Props.SetText(ical.PropUID, uid)
	vevent.Props.SetText(ical.PropSummary, e.Title)
	if e.Description != "" {
		vevent.Props.SetText(ical.PropDescription, e.Description)
	}
	if e.Location != "" {
		vevent.Props.SetText(ical.PropLocation, e.Location)
	}

	if e.AllDay {
		dtStart := ical.NewProp(ical.PropDateTimeStart)
		dtStart.SetDate(e.StartTime)
		vevent.Props.Set(dtStart)

		dtEnd := ical.NewProp(ical.PropDateTimeEnd)
		dtEnd.SetDate(e.EndTime.AddDate(0, 0, 1))
		vevent.Props.Set(dtEnd)
	} else if e.Timezone != "" {
		if loc, err := time.LoadLocation(e.Timezone); err == nil {
			dtStart := ical.NewProp(ical.PropDateTimeStart)
			dtStart.Params.Set("TZID", e.Timezone)
			dtStart.SetValueType(ical.ValueDateTime)
			dtStart.Value = e.StartTime.In(loc).Format("20060102T150405")
			vevent.Props.Set(dtStart)

			dtEnd := ical.NewProp(ical.PropDateTimeEnd)
			dtEnd.Params.Set("TZID", e.Timezone)
			dtEnd.SetValueType(ical.ValueDateTime)
			dtEnd.Value = e.EndTime.In(loc).Format("20060102T150405")
			vevent.Props.Set(dtEnd)
		}
	} else {
		dtStart := ical.NewProp(ical.PropDateTimeStart)
		dtStart.SetDateTime(e.StartTime)
		vevent.Props.Set(dtStart)

		dtEnd := ical.NewProp(ical.PropDateTimeEnd)
		dtEnd.SetDateTime(e.EndTime)
		vevent.Props.Set(dtEnd)
	}

	dtstamp := ical.NewProp(ical.PropDateTimeStamp)
	dtstamp.SetDateTime(e.UpdatedAt)
	vevent.Props.Set(dtstamp)

	cal.Children = append(cal.Children, vevent)
	return cal
}

func eventToCalendarObject(e *Event, basePath string) *caldav.CalendarObject {
	uid := e.RemoteUID
	if uid == "" {
		uid = e.ID + "@localitas"
	}
	cal := eventToICalendar(e)
	etag := fmt.Sprintf("%x", sha256.Sum256([]byte(uid+e.UpdatedAt.String())))
	return &caldav.CalendarObject{
		Path:    basePath + uid + ".ics",
		ModTime: e.UpdatedAt,
		ETag:    `"` + etag[:16] + `"`,
		Data:    cal,
	}
}

func eventsToCalendarObjects(events []*Event, basePath string) []caldav.CalendarObject {
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	out := make([]caldav.CalendarObject, 0, len(events))
	for _, e := range events {
		out = append(out, *eventToCalendarObject(e, basePath))
	}
	return out
}
