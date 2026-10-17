package calendar

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/localitas/localitas-go"
)

// go-webdav places resources by depth under the prefix: principal, home
// set, calendar, object. The principal is always the verified caller, so it
// is spelled "me".
const (
	caldavPrefix    = "/caldav/"
	caldavPrincipal = caldavPrefix + "me/"
	caldavHome      = caldavPrincipal + "calendars/"
)

func calendarPath(id string) string {
	return caldavHome + id + "/"
}

type CalDAVBackend struct {
	store *Store
}

func NewCalDAVBackend(store *Store) *CalDAVBackend {
	return &CalDAVBackend{store: store}
}

// NewCalDAVHandler serves the CalDAV tree for the user core vouches for: a
// request without a verified user is refused, every lookup sees only that
// user's calendars, and changing anything needs write scope.
func NewCalDAVHandler(store *Store, prefix string) http.Handler {
	dav := &caldav.Handler{
		Backend: NewCalDAVBackend(store),
		Prefix:  prefix,
	}
	writer := client.RequireScope(client.ScopeWrite)(dav)
	return client.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, "PROPFIND", "REPORT":
			dav.ServeHTTP(w, r)
		default:
			writer.ServeHTTP(w, r)
		}
	}))
}

// caldavUser is the verified user of a CalDAV request.
func caldavUser(ctx context.Context) (string, error) {
	userID := client.GetUserID(ctx)
	if userID == "" {
		return "", webdav.NewHTTPError(http.StatusUnauthorized, errors.New("no verified user"))
	}
	return userID, nil
}

// davErr turns a record the caller does not own into a CalDAV 404.
func davErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return err
}

func (b *CalDAVBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return caldavPrincipal, nil
}

func (b *CalDAVBackend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return caldavHome, nil
}

// calendarAt returns the caller's calendar a CalDAV path names.
func (b *CalDAVBackend) calendarAt(ctx context.Context, path string) (string, *Calendar, error) {
	userID, err := caldavUser(ctx)
	if err != nil {
		return "", nil, err
	}
	segment := extractCalendarID(path)
	if segment == "" {
		return "", nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("calendar not found"))
	}
	cal, err := b.store.GetCalendarByPath(ctx, userID, segment)
	if err != nil {
		return "", nil, davErr(err)
	}
	return userID, cal, nil
}

func davCalendar(c *Calendar) caldav.Calendar {
	return caldav.Calendar{
		Path:                  calendarPath(c.ID),
		Name:                  c.Name,
		Description:           c.Name,
		SupportedComponentSet: []string{"VEVENT"},
	}
}

func (b *CalDAVBackend) CreateCalendar(ctx context.Context, calendar *caldav.Calendar) error {
	userID, err := caldavUser(ctx)
	if err != nil {
		return err
	}
	local, err := b.store.EnsureLocalCalendar(ctx, userID)
	if err != nil {
		return err
	}
	_, err = b.store.UpsertCalendar(ctx, local.AccountID, calendar.Name, calendar.Path, "#7d6b96")
	return err
}

func (b *CalDAVBackend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	userID, err := caldavUser(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := b.store.EnsureLocalCalendar(ctx, userID); err != nil {
		return nil, err
	}
	cals, err := b.store.ListCalendars(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	out := make([]caldav.Calendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, davCalendar(c))
	}
	return out, nil
}

func (b *CalDAVBackend) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	_, c, err := b.calendarAt(ctx, path)
	if err != nil {
		return nil, err
	}
	dc := davCalendar(c)
	return &dc, nil
}

// objectAt returns the caller's event a CalDAV object path names. Events
// stored without a remote UID are addressed as <id>@localitas.
func (b *CalDAVBackend) objectAt(ctx context.Context, path string) (string, *Calendar, *Event, error) {
	userID, cal, err := b.calendarAt(ctx, path)
	if err != nil {
		return "", nil, nil, err
	}
	uid := extractEventUID(path)
	if uid == "" {
		return "", nil, nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("event not found"))
	}
	e, err := b.store.GetEventByRemoteUID(ctx, userID, cal.ID, uid)
	if errors.Is(err, ErrNotFound) && strings.HasSuffix(uid, "@localitas") {
		e, err = b.store.Get(ctx, userID, strings.TrimSuffix(uid, "@localitas"))
		if err == nil && (e.CalendarID != cal.ID || e.RemoteUID != "") {
			err = ErrNotFound
		}
	}
	if err != nil {
		return "", nil, nil, davErr(err)
	}
	return userID, cal, e, nil
}

func (b *CalDAVBackend) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	_, cal, e, err := b.objectAt(ctx, path)
	if err != nil {
		return nil, err
	}
	return eventToCalendarObject(e, calendarPath(cal.ID)), nil
}

func (b *CalDAVBackend) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	userID, cal, err := b.calendarAt(ctx, path)
	if err != nil {
		return nil, err
	}
	events, err := b.store.GetEventsByCalendarID(ctx, userID, cal.ID)
	if err != nil {
		return nil, err
	}
	return eventsToCalendarObjects(events, calendarPath(cal.ID)), nil
}

func (b *CalDAVBackend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	userID, cal, err := b.calendarAt(ctx, path)
	if err != nil {
		return nil, err
	}
	var events []*Event
	if !query.CompFilter.Start.IsZero() && !query.CompFilter.End.IsZero() {
		events, err = b.store.GetEventsByCalendarIDRange(ctx, userID, cal.ID, query.CompFilter.Start, query.CompFilter.End)
	} else {
		events, err = b.store.GetEventsByCalendarID(ctx, userID, cal.ID)
	}
	if err != nil {
		return nil, err
	}
	return eventsToCalendarObjects(events, calendarPath(cal.ID)), nil
}

func (b *CalDAVBackend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	userID, cal, err := b.calendarAt(ctx, path)
	if err != nil {
		return nil, err
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
			return nil, webdav.NewHTTPError(http.StatusBadRequest, fmt.Errorf("DTSTART: %w", err))
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

		existing, err := b.store.GetEventByRemoteUID(ctx, userID, cal.ID, uid)
		switch {
		case err == nil:
			if err := b.store.Update(ctx, userID, existing.ID, summary, description, location, dtStart, dtEnd, allDay); err != nil {
				return nil, davErr(err)
			}
		case errors.Is(err, ErrNotFound):
			if _, err := b.store.Create(ctx, cal, uid, summary, description, location, tz, dtStart, dtEnd, allDay); err != nil {
				return nil, err
			}
		default:
			return nil, err
		}

		e, err := b.store.GetEventByRemoteUID(ctx, userID, cal.ID, uid)
		if err != nil {
			return nil, davErr(err)
		}
		return eventToCalendarObject(e, calendarPath(cal.ID)), nil
	}

	return nil, webdav.NewHTTPError(http.StatusBadRequest, errors.New("no event found in calendar data"))
}

func (b *CalDAVBackend) DeleteCalendarObject(ctx context.Context, path string) error {
	userID, _, e, err := b.objectAt(ctx, path)
	if err != nil {
		return err
	}
	return davErr(b.store.Delete(ctx, userID, e.ID))
}

func (b *CalDAVBackend) GetEventsByDay(ctx context.Context, userID string, start, end time.Time) ([]DayEvents, error) {
	return b.store.GetEventsByDay(ctx, userID, start, end)
}

func (b *CalDAVBackend) GetEvent(ctx context.Context, userID, id string) (*Event, error) {
	return b.store.Get(ctx, userID, id)
}

// CreateEventJSON adds an event to userID's calendar calendarID (their local
// calendar when empty) and pushes it to the remote calendar behind it.
func (b *CalDAVBackend) CreateEventJSON(ctx context.Context, userID, calendarID, title, description, location, timezone string, start, end time.Time, allDay bool) (*Event, error) {
	var cal *Calendar
	var err error
	if calendarID == "" {
		cal, err = b.store.EnsureLocalCalendar(ctx, userID)
	} else {
		cal, err = b.store.GetCalendar(ctx, userID, calendarID)
	}
	if err != nil {
		return nil, err
	}
	uid := newEventUID()
	e, err := b.store.Create(ctx, cal, uid, title, description, location, timezone, start, end, allDay)
	if err != nil {
		return nil, err
	}
	b.pushToRemote(ctx, userID, cal, uid, title, description, location, start, end, allDay)
	return b.store.Get(ctx, userID, e.ID)
}

// UpdateEventJSON rewrites userID's event id and pushes it to the remote
// calendar behind it.
func (b *CalDAVBackend) UpdateEventJSON(ctx context.Context, userID, id, title, description, location string, start, end time.Time, allDay bool) (*Event, error) {
	existing, err := b.store.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := b.store.Update(ctx, userID, id, title, description, location, start, end, allDay); err != nil {
		return nil, err
	}
	if existing.RemoteUID != "" {
		cal, err := b.store.GetCalendar(ctx, userID, existing.CalendarID)
		if err != nil {
			return nil, err
		}
		b.pushToRemote(ctx, userID, cal, existing.RemoteUID, title, description, location, start, end, allDay)
	}
	return b.store.Get(ctx, userID, id)
}

// DeleteEventJSON removes userID's event id, and its remote copy.
func (b *CalDAVBackend) DeleteEventJSON(ctx context.Context, userID, id string) error {
	existing, err := b.store.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	if existing.RemoteUID != "" {
		cal, err := b.store.GetCalendar(ctx, userID, existing.CalendarID)
		if err != nil {
			return err
		}
		remote, err := remoteClientForCalendar(ctx, b.store, userID, cal)
		if err != nil {
			logger.Error("remote delete skipped", "event_id", existing.ID, "error", err)
		} else if remote != nil {
			if err := remote.DeleteEvent(ctx, existing.RemoteUID); err != nil {
				logger.Error("remote delete failed", "event_id", existing.ID, "error", err)
			}
		}
	}
	return b.store.Delete(ctx, userID, id)
}

func (b *CalDAVBackend) SearchEvents(ctx context.Context, userID, query string, limit int) ([]*Event, error) {
	return b.store.Search(ctx, userID, query, limit)
}

// pushToRemote writes an event to the remote calendar behind cal. The local
// write has already succeeded, so a failed push is logged rather than
// failing the request; the next sync reconciles.
func (b *CalDAVBackend) pushToRemote(ctx context.Context, userID string, cal *Calendar, uid, title, description, location string, start, end time.Time, allDay bool) {
	remote, err := remoteClientForCalendar(ctx, b.store, userID, cal)
	if err != nil {
		logger.Error("remote push skipped", "calendar_id", cal.ID, "error", err)
		return
	}
	if remote == nil {
		return
	}
	if err := remote.UpdateEvent(ctx, uid, title, description, location, start, end, allDay); err != nil {
		logger.Error("remote push failed", "calendar_id", cal.ID, "uid", uid, "error", err)
	}
}

func extractCalendarID(path string) string {
	if !strings.HasPrefix(path, caldavHome) {
		return ""
	}
	id, _, _ := strings.Cut(strings.TrimPrefix(path, caldavHome), "/")
	return id
}

func extractEventUID(path string) string {
	if !strings.HasPrefix(path, caldavHome) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, caldavHome), "/")
	if len(parts) < 2 || parts[1] == "" {
		return ""
	}
	return strings.TrimSuffix(parts[1], ".ics")
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
