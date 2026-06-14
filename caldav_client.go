package calendar

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
)

type CalDAVClient struct {
	baseURL     string
	username    string
	password    string
	bearerToken string
	client      *http.Client
}

func NewCalDAVClient(baseURL, username, password string) *CalDAVClient {
	return &CalDAVClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func NewCalDAVClientWithBearer(baseURL, bearerToken string) *CalDAVClient {
	return &CalDAVClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		bearerToken: bearerToken,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

type calDAVCalendar struct {
	Href        string
	DisplayName string
	Color       string
}

func (c *CalDAVClient) DiscoverCalendars(ctx context.Context) ([]calDAVCalendar, error) {
	propfindXML := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:resourcetype />
    <d:displayname />
  </d:prop>
</d:propfind>`

	req, err := http.NewRequestWithContext(ctx, "PROPFIND", c.baseURL, strings.NewReader(propfindXML))
	if err != nil {
		return nil, err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 207 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("propfind failed: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var ms multiStatus
	xml.Unmarshal(body, &ms)

	cals := make([]calDAVCalendar, 0)
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Prop.ResourceType, "calendar") {
				name := ps.Prop.DisplayName
				if name == "" {
					name = r.Href
				}
				cals = append(cals, calDAVCalendar{Href: r.Href, DisplayName: name})
			}
		}
	}
	return cals, nil
}

func (c *CalDAVClient) FetchEventsFromURL(ctx context.Context, calURL string, from, to time.Time) ([]calDAVEvent, error) {
	reportXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:getetag />
    <c:calendar-data />
  </d:prop>
  <c:filter>
    <c:comp-filter name="VCALENDAR">
      <c:comp-filter name="VEVENT">
        <c:time-range start="%s" end="%s"/>
      </c:comp-filter>
    </c:comp-filter>
  </c:filter>
</c:calendar-query>`, from.UTC().Format("20060102T150405Z"), to.UTC().Format("20060102T150405Z"))

	req, err := http.NewRequestWithContext(ctx, "REPORT", calURL, strings.NewReader(reportXML))
	if err != nil {
		return nil, err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed (401)")
	}
	if resp.StatusCode != 207 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("report %d", resp.StatusCode)
	}

	return parseMultiStatus(resp.Body)
}

type calDAVEvent struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Recurrence  string
	Timezone    string
}

func (c *CalDAVClient) FetchEvents(ctx context.Context, from, to time.Time) ([]calDAVEvent, error) {
	reportXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:getetag />
    <c:calendar-data />
  </d:prop>
  <c:filter>
    <c:comp-filter name="VCALENDAR">
      <c:comp-filter name="VEVENT">
        <c:time-range start="%s" end="%s"/>
      </c:comp-filter>
    </c:comp-filter>
  </c:filter>
</c:calendar-query>`, from.UTC().Format("20060102T150405Z"), to.UTC().Format("20060102T150405Z"))

	req, err := http.NewRequestWithContext(ctx, "REPORT", c.baseURL, strings.NewReader(reportXML))
	if err != nil {
		return nil, err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("caldav request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401)")
	}
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("caldav response %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	return parseMultiStatus(resp.Body)
}

type multiStatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string     `xml:"href"`
	Propstat []propStat `xml:"propstat"`
}

type propStat struct {
	Prop   davProp `xml:"prop"`
	Status string  `xml:"status"`
}

type davProp struct {
	CalendarData string `xml:"calendar-data"`
	ETag         string `xml:"getetag"`
	ResourceType string `xml:"resourcetype"`
	DisplayName  string `xml:"displayname"`
}

func parseMultiStatus(r io.Reader) ([]calDAVEvent, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var ms multiStatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, fmt.Errorf("parse multistatus: %w", err)
	}

	events := make([]calDAVEvent, 0)
	for _, resp := range ms.Responses {
		for _, ps := range resp.Propstat {
			if ps.Prop.CalendarData == "" {
				continue
			}
			parsed, err := parseICS(ps.Prop.CalendarData)
			if err != nil {
				continue
			}
			events = append(events, parsed...)
		}
	}
	return events, nil
}

func parseICS(icsData string) ([]calDAVEvent, error) {
	dec := ical.NewDecoder(strings.NewReader(icsData))
	cal, err := dec.Decode()
	if err != nil {
		return nil, err
	}

	events := make([]calDAVEvent, 0)
	for _, comp := range cal.Children {
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
		if startProp := event.Props.Get(ical.PropDateTimeStart); startProp != nil {
			if v := startProp.Params.Get("VALUE"); v == "DATE" {
				allDay = true
			}
		}

		var rruleStr string
		if rruleProp := event.Props.Get(ical.PropRecurrenceRule); rruleProp != nil {
			rruleStr = rruleProp.Value
		}

		var tz string
		if startProp := event.Props.Get(ical.PropDateTimeStart); startProp != nil {
			if tzid := startProp.Params.Get("TZID"); tzid != "" {
				tz = tzid
			}
		}

		events = append(events, calDAVEvent{
			UID:         uid,
			Summary:     summary,
			Description: description,
			Location:    location,
			Start:       dtStart,
			End:         dtEnd,
			AllDay:      allDay,
			Recurrence:  rruleStr,
			Timezone:    tz,
		})
	}
	return events, nil
}

func generateICS(uid, summary, description, location, tz string, start, end time.Time, allDay bool) string {
	dtFmt := "20060102T150405Z"
	localFmt := "20060102T150405"
	dtPropStart := "DTSTART:" + start.UTC().Format(dtFmt)
	dtPropEnd := "DTEND:" + end.UTC().Format(dtFmt)
	if allDay {
		dtPropStart = "DTSTART;VALUE=DATE:" + start.Format("20060102")
		dtPropEnd = "DTEND;VALUE=DATE:" + end.AddDate(0, 0, 1).Format("20060102")
	} else if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			dtPropStart = "DTSTART;TZID=" + tz + ":" + start.In(loc).Format(localFmt)
			dtPropEnd = "DTEND;TZID=" + tz + ":" + end.In(loc).Format(localFmt)
		}
	}
	now := time.Now().UTC().Format(dtFmt)
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Localitas//Calendar//EN",
		"BEGIN:VEVENT",
		"UID:" + uid,
		dtPropStart,
		dtPropEnd,
		"DTSTAMP:" + now,
		"SUMMARY:" + summary,
	}
	if description != "" {
		lines = append(lines, "DESCRIPTION:"+description)
	}
	if location != "" {
		lines = append(lines, "LOCATION:"+location)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	return strings.Join(lines, "\r\n") + "\r\n"
}

func (c *CalDAVClient) CreateEvent(ctx context.Context, uid, summary, description, location string, start, end time.Time, allDay bool) error {
	icsData := generateICS(uid, summary, description, location, "", start, end, allDay)
	eventURL := strings.TrimRight(c.baseURL, "/") + "/" + uid + ".ics"

	req, err := http.NewRequestWithContext(ctx, "PUT", eventURL, strings.NewReader(icsData))
	if err != nil {
		return err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	req.Header.Set("If-None-Match", "*")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("caldav put: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caldav create event %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return nil
}

func (c *CalDAVClient) UpdateEvent(ctx context.Context, uid, summary, description, location string, start, end time.Time, allDay bool) error {
	icsData := generateICS(uid, summary, description, location, "", start, end, allDay)
	eventURL := strings.TrimRight(c.baseURL, "/") + "/" + uid + ".ics"

	req, err := http.NewRequestWithContext(ctx, "PUT", eventURL, strings.NewReader(icsData))
	if err != nil {
		return err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("caldav update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caldav update event %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return nil
}

func (c *CalDAVClient) DeleteEvent(ctx context.Context, uid string) error {
	eventURL := strings.TrimRight(c.baseURL, "/") + "/" + uid + ".ics"

	req, err := http.NewRequestWithContext(ctx, "DELETE", eventURL, nil)
	if err != nil {
		return err
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("caldav delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caldav delete event %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}
	return nil
}

func getCalDAVClientForEvent(ctx context.Context, store *Store, event *Event) (*CalDAVClient, error) {
	if event.CalendarID == "" || event.RemoteUID == "" {
		return nil, nil
	}
	cal, err := store.GetCalendar(ctx, event.CalendarID)
	if err != nil {
		return nil, nil
	}
	account, err := store.GetAccount(ctx, cal.AccountID)
	if err != nil {
		return nil, nil
	}
	if account.NeedsOAuth() {
		fullAcct, err := store.GetAccountWithTokens(ctx, account.ID)
		if err != nil {
			return nil, fmt.Errorf("get tokens: %w", err)
		}
		if err := EnsureValidToken(ctx, store, fullAcct); err != nil {
			return nil, fmt.Errorf("oauth: %w", err)
		}
		return NewCalDAVClientWithBearer(cal.Href, fullAcct.AccessToken), nil
	}
	if account.CalDAVURL != "" {
		return NewCalDAVClient(account.CalDAVURL, account.Username, account.Password), nil
	}
	return nil, nil
}

func SyncAccount(ctx context.Context, store *Store, account *CalendarAccount, from, to time.Time) (int, error) {
	store.db.ExecContext(ctx, `
		DELETE FROM events WHERE rowid NOT IN (
			SELECT MIN(rowid) FROM events WHERE account_id = ? AND remote_uid != '' GROUP BY remote_uid
		) AND account_id = ? AND remote_uid != ''`, account.ID, account.ID)

	store.db.ExecContext(ctx, `
		DELETE FROM calendars WHERE rowid NOT IN (
			SELECT MIN(rowid) FROM calendars WHERE account_id = ? GROUP BY name
		) AND account_id = ?`, account.ID, account.ID)

	type calWithEvents struct {
		calendarID string
		events     []calDAVEvent
	}

	var results []calWithEvents
	calColors := []string{"#7d6b96", "#e88b8b", "#60a5fa", "#a8d5ba", "#f5e6a8", "#c5b3e0", "#f0a8c8", "#8bc5d5"}

	if account.NeedsOAuth() {
		fullAcct, err := store.GetAccountWithTokens(ctx, account.ID)
		if err != nil {
			return 0, fmt.Errorf("get tokens: %w", err)
		}
		if err := EnsureValidToken(ctx, store, fullAcct); err != nil {
			return 0, fmt.Errorf("oauth: %w", err)
		}

		cals, err := DiscoverGoogleCalendars(ctx, fullAcct.AccessToken)
		if err != nil {
			return 0, fmt.Errorf("discover calendars: %w", err)
		}

		for i, cal := range cals {
			color := cal.Color
			if color == "" {
				color = calColors[i%len(calColors)]
			}
			stored, uErr := store.UpsertCalendar(ctx, account.ID, cal.DisplayName, cal.Href, color)
			if uErr != nil {
				continue
			}
			caldavClient := NewCalDAVClientWithBearer(cal.Href, fullAcct.AccessToken)
			events, fErr := caldavClient.FetchEvents(ctx, from, to)
			if fErr != nil {
				continue
			}
			results = append(results, calWithEvents{calendarID: stored.ID, events: events})
		}
	} else {
		caldavClient := NewCalDAVClient(account.CalDAVURL, account.Username, account.Password)

		cals, err := caldavClient.DiscoverCalendars(ctx)
		if err == nil && len(cals) > 0 {
			for i, cal := range cals {
				calURL := cal.Href
				if !strings.HasPrefix(calURL, "http") {
					base := caldavClient.baseURL
					if idx := strings.Index(base, "://"); idx >= 0 {
						slashIdx := strings.Index(base[idx+3:], "/")
						if slashIdx >= 0 {
							base = base[:idx+3+slashIdx]
						}
					}
					calURL = base + cal.Href
				}
				color := cal.Color
				if color == "" {
					color = calColors[i%len(calColors)]
				}
				stored, uErr := store.UpsertCalendar(ctx, account.ID, cal.DisplayName, cal.Href, color)
				if uErr != nil {
					continue
				}
				events, fErr := caldavClient.FetchEventsFromURL(ctx, calURL, from, to)
				if fErr != nil {
					continue
				}
				results = append(results, calWithEvents{calendarID: stored.ID, events: events})
			}
		} else {
			stored, uErr := store.UpsertCalendar(ctx, account.ID, account.Name, account.CalDAVURL, account.Color)
			calID := ""
			if uErr == nil {
				calID = stored.ID
			}
			events, fErr := caldavClient.FetchEvents(ctx, from, to)
			if fErr != nil {
				return 0, fErr
			}
			results = append(results, calWithEvents{calendarID: calID, events: events})
		}
	}

	newCount := 0
	seen := make(map[string]bool)
	for _, r := range results {
		for _, re := range r.events {
			if re.UID == "" || seen[re.UID] {
				continue
			}
			seen[re.UID] = true

			now := time.Now().UTC().Unix()
			allDayInt := 0
			if re.AllDay {
				allDayInt = 1
			}

			var existingID string
			err := store.db.QueryRowContext(ctx, "SELECT id FROM events WHERE account_id = ? AND remote_uid = ?", account.ID, re.UID).Scan(&existingID)
			if err == nil {
				store.db.ExecContext(ctx, `
					UPDATE events SET calendar_id=?, title=?, description=?, start_time=?, end_time=?, all_day=?, location=?, recurrence=?, timezone=?, updated_at=?
					WHERE id=?`,
					r.calendarID, re.Summary, re.Description, re.Start.Unix(), re.End.Unix(), allDayInt, re.Location, re.Recurrence, re.Timezone, now, existingID)
			} else {
				id := newEventID()
				_, err = store.db.ExecContext(ctx, `
					INSERT INTO events (id, account_id, calendar_id, remote_uid, title, description, start_time, end_time, all_day, location, recurrence, timezone, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, account.ID, r.calendarID, re.UID, re.Summary, re.Description, re.Start.Unix(), re.End.Unix(), allDayInt, re.Location, re.Recurrence, re.Timezone, now, now)
				if err == nil {
					newCount++
				}
			}
		}
	}

	return newCount, nil
}
