package calendar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/localitas/localitas-go"
	"github.com/teambition/rrule-go"
)

const DatabaseName = "calendar"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func OpenStore(coreURL, dbID, token string) (*Store, error) {
	dsn := fmt.Sprintf("%s?database_id=%s&token=%s", coreURL, dbID, token)
	db, err := sql.Open("localitas", dsn)
	if err != nil {
		return nil, fmt.Errorf("open localitas db: %w", err)
	}
	return NewStore(db), nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateAccount(ctx context.Context, userID, name, provider, email, caldavURL, username, password, oauthClientID, oauthClientSecret, color, vaultCredentialID string) (*CalendarAccount, error) {
	id := newEventID()
	now := time.Now().UTC().Unix()
	if color == "" {
		color = "#7d6b96"
	}
	encPass, _ := client.Encrypt(password)
	encSecret, _ := client.Encrypt(oauthClientSecret)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (id, user_id, name, provider, email, caldav_url, username, password, oauth_client_id, oauth_client_secret, color, vault_credential_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, name, provider, email, caldavURL, username, encPass, oauthClientID, encSecret, color, vaultCredentialID, now, now)
	if err != nil {
		return nil, err
	}
	return &CalendarAccount{ID: id, Name: name, Provider: provider, Email: email, CalDAVURL: caldavURL, Username: username, OAuthClientID: oauthClientID, VaultCredentialID: vaultCredentialID, Color: color, IsActive: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (s *Store) ListAccounts(ctx context.Context, userID string) ([]*CalendarAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, provider, email, caldav_url, username, color, vault_credential_id, is_active, last_synced_at, COALESCE(sync_error,''), created_at, updated_at FROM accounts WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*CalendarAccount, 0)
	for rows.Next() {
		var a CalendarAccount
		var active int
		var lastSynced *int64
		var createdAt, updatedAt int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &a.Email, &a.CalDAVURL, &a.Username, &a.Color, &a.VaultCredentialID, &active, &lastSynced, &a.SyncError, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		a.IsActive = active == 1
		a.LastSyncedAt = lastSynced
		a.CreatedAt = time.Unix(createdAt, 0)
		a.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &a)
	}
	return out, nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (*CalendarAccount, error) {
	var a CalendarAccount
	var active int
	var lastSynced *int64
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, provider, email, caldav_url, username, password, COALESCE(oauth_client_id,''), COALESCE(oauth_client_secret,''), color, COALESCE(vault_credential_id,''), is_active, last_synced_at, COALESCE(sync_error,''), created_at, updated_at FROM accounts WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Provider, &a.Email, &a.CalDAVURL, &a.Username, &a.Password, &a.OAuthClientID, &a.OAuthClientSecret, &a.Color, &a.VaultCredentialID, &active, &lastSynced, &a.SyncError, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	a.IsActive = active == 1
	a.LastSyncedAt = lastSynced
	a.CreatedAt = time.Unix(createdAt, 0)
	a.UpdatedAt = time.Unix(updatedAt, 0)
	a.Password, _ = client.Decrypt(a.Password)
	a.OAuthClientSecret, _ = client.Decrypt(a.OAuthClientSecret)
	return &a, nil
}

func (s *Store) UpdateAccount(ctx context.Context, id, name, provider, email, caldavURL, username, password, color, vaultCredentialID string) error {
	now := time.Now().UTC().Unix()
	encPass, _ := client.Encrypt(password)
	_, err := s.db.ExecContext(ctx,
		`UPDATE accounts SET name=?, provider=?, email=?, caldav_url=?, username=?, password=?, color=?, vault_credential_id=?, updated_at=? WHERE id=?`,
		name, provider, email, caldavURL, username, encPass, color, vaultCredentialID, now, id)
	return err
}

func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	s.db.ExecContext(ctx, "DELETE FROM events WHERE account_id = ?", id)
	s.db.ExecContext(ctx, "DELETE FROM calendars WHERE account_id = ?", id)
	_, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	return err
}

func (s *Store) ClearAccountData(ctx context.Context, accountID string) {
	s.db.ExecContext(ctx, "DELETE FROM events WHERE account_id = ?", accountID)
	s.db.ExecContext(ctx, "DELETE FROM calendars WHERE account_id = ?", accountID)
}

func (s *Store) UpdateSyncStatus(ctx context.Context, id, syncErr string) {
	now := time.Now().UTC().Unix()
	s.db.ExecContext(ctx, "UPDATE accounts SET last_synced_at = ?, sync_error = ?, updated_at = ? WHERE id = ?", now, syncErr, now, id)
}

func (s *Store) UpsertCalendar(ctx context.Context, accountID, name, href, color string) (*Calendar, error) {
	var existing string
	var existingColor string
	err := s.db.QueryRowContext(ctx, "SELECT id, color FROM calendars WHERE account_id = ? AND (href = ? OR name = ?)", accountID, href, name).Scan(&existing, &existingColor)
	if err == nil {
		now := time.Now().UTC().Unix()
		s.db.ExecContext(ctx, "UPDATE calendars SET name = ?, href = ?, updated_at = ? WHERE id = ?", name, href, now, existing)
		return &Calendar{ID: existing, AccountID: accountID, Name: name, Href: href, Color: existingColor, IsVisible: true}, nil
	}
	id := newEventID()
	now := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO calendars (id, account_id, name, href, color, is_visible, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
		id, accountID, name, href, color, now, now)
	if err != nil {
		return nil, err
	}
	return &Calendar{ID: id, AccountID: accountID, Name: name, Href: href, Color: color, IsVisible: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func (s *Store) ListCalendars(ctx context.Context, accountID string) ([]*Calendar, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, account_id, name, href, color, is_visible, created_at, updated_at FROM calendars WHERE account_id = ? ORDER BY name", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Calendar, 0)
	for rows.Next() {
		var c Calendar
		var visible int
		var createdAt, updatedAt int64
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Href, &c.Color, &visible, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.IsVisible = visible == 1
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &c)
	}
	return out, nil
}

func (s *Store) ListAllCalendars(ctx context.Context) ([]*Calendar, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, account_id, name, href, color, is_visible, created_at, updated_at FROM calendars ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Calendar, 0)
	for rows.Next() {
		var c Calendar
		var visible int
		var createdAt, updatedAt int64
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Href, &c.Color, &visible, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.IsVisible = visible == 1
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &c)
	}
	return out, nil
}

func (s *Store) UpdateCalendarVisibility(ctx context.Context, id string, visible bool) error {
	v := 0
	if visible {
		v = 1
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE calendars SET is_visible = ?, updated_at = ? WHERE id = ?", v, now, id)
	return err
}

func (s *Store) GetCalendar(ctx context.Context, id string) (*Calendar, error) {
	var c Calendar
	var visible int
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, account_id, name, href, color, is_visible, created_at, updated_at FROM calendars WHERE id = ?", id).
		Scan(&c.ID, &c.AccountID, &c.Name, &c.Href, &c.Color, &visible, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.IsVisible = visible == 1
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	return &c, nil
}

func (s *Store) UpdateCalendarColor(ctx context.Context, id, color string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx, "UPDATE calendars SET color = ?, updated_at = ? WHERE id = ?", color, now, id)
	return err
}

func (s *Store) Create(ctx context.Context, calendarID, accountID, remoteUID, title, description, location, timezone string, startTime, endTime time.Time, allDay bool) (*Event, error) {
	id := newEventID()
	now := time.Now().UTC().Unix()
	allDayInt := 0
	if allDay {
		allDayInt = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, calendar_id, account_id, remote_uid, title, description, start_time, end_time, all_day, location, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, calendarID, accountID, remoteUID, title, description, startTime.Unix(), endTime.Unix(), allDayInt, location, timezone, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}

	return &Event{
		ID: id, CalendarID: calendarID, AccountID: accountID, RemoteUID: remoteUID,
		Title: title, Description: description,
		StartTime: startTime, EndTime: endTime, AllDay: allDay,
		Location: location, Timezone: timezone,
		CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0),
	}, nil
}

func (s *Store) Get(ctx context.Context, id string) (*Event, error) {
	var e Event
	var startTime, endTime, createdAt, updatedAt int64
	var allDay int
	err := s.db.QueryRowContext(ctx, `
		SELECT e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
			COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(c.color, a.color, '#7d6b96'), COALESCE(e.recurrence,''), COALESCE(e.timezone,'')
		FROM events e
		LEFT JOIN calendars c ON e.calendar_id = c.id
		LEFT JOIN accounts a ON e.account_id = a.id
		WHERE e.id = ?`, id,
	).Scan(&e.ID, &e.Title, &e.Description, &startTime, &endTime, &allDay, &e.Location, &createdAt, &updatedAt, &e.AccountID, &e.CalendarID, &e.Color, &e.Recurrence, &e.Timezone)
	if err != nil {
		return nil, fmt.Errorf("event %s not found", id)
	}
	e.StartTime = time.Unix(startTime, 0)
	e.EndTime = time.Unix(endTime, 0)
	e.AllDay = allDay == 1
	e.CreatedAt = time.Unix(createdAt, 0)
	e.UpdatedAt = time.Unix(updatedAt, 0)
	return &e, nil
}

func (s *Store) GetEventsByRange(ctx context.Context, start, end time.Time) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
			COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(c.color, a.color, '#7d6b96'), COALESCE(e.recurrence,''), COALESCE(e.timezone,'')
		FROM events e
		LEFT JOIN calendars c ON e.calendar_id = c.id
		LEFT JOIN accounts a ON e.account_id = a.id
		WHERE ((e.start_time <= ? AND e.end_time >= ?) OR e.recurrence != '')
			AND (c.id IS NULL OR c.is_visible = 1)
		ORDER BY e.start_time ASC`,
		end.Unix(), start.Unix())
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	return expandRecurringEvents(events, start, end), nil
}

func (s *Store) GetEventsByDay(ctx context.Context, start, end time.Time) ([]DayEvents, error) {
	events, err := s.GetEventsByRange(ctx, start, end)
	if err != nil {
		return nil, err
	}

	dayMap := make(map[string]*DayEvents)
	var days []DayEvents

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		de := &DayEvents{
			Date:      key,
			DateLabel: d.Format("Monday, January 2"),
			Events:    []*Event{},
		}
		dayMap[key] = de
		days = append(days, *de)
	}

	for _, e := range events {
		eventStart := e.StartTime
		eventEnd := e.EndTime
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
			dayEnd := dayStart.Add(24*time.Hour - time.Second)
			if eventStart.Before(dayEnd) && eventEnd.After(dayStart) {
				key := d.Format("2006-01-02")
				if de, ok := dayMap[key]; ok {
					de.Events = append(de.Events, e)
				}
			}
		}
	}

	result := make([]DayEvents, 0, len(days))
	for _, d := range days {
		if de, ok := dayMap[d.Date]; ok {
			result = append(result, *de)
		}
	}
	return result, nil
}

func (s *Store) Update(ctx context.Context, id, title, description, location string, startTime, endTime time.Time, allDay bool) error {
	now := time.Now().UTC().Unix()
	allDayInt := 0
	if allDay {
		allDayInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE events SET title=?, description=?, start_time=?, end_time=?, all_day=?, location=?, updated_at=?
		WHERE id=?`,
		title, description, startTime.Unix(), endTime.Unix(), allDayInt, location, now, id)
	return err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id)
	return err
}

func (s *Store) GetEventByRemoteUID(ctx context.Context, remoteUID string) (*Event, error) {
	var e Event
	var startTime, endTime, createdAt, updatedAt int64
	var allDay int
	err := s.db.QueryRowContext(ctx, `
		SELECT e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
			COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(e.remote_uid,'')
		FROM events e
		WHERE e.remote_uid = ?`, remoteUID,
	).Scan(&e.ID, &e.Title, &e.Description, &startTime, &endTime, &allDay, &e.Location, &createdAt, &updatedAt, &e.AccountID, &e.CalendarID, &e.RemoteUID)
	if err != nil {
		return nil, err
	}
	e.StartTime = time.Unix(startTime, 0)
	e.EndTime = time.Unix(endTime, 0)
	e.AllDay = allDay == 1
	e.CreatedAt = time.Unix(createdAt, 0)
	e.UpdatedAt = time.Unix(updatedAt, 0)
	return &e, nil
}

func (s *Store) GetEventsByCalendarID(ctx context.Context, calendarID string) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, description, start_time, end_time, all_day, location, created_at, updated_at,
			COALESCE(account_id,''), COALESCE(calendar_id,''), COALESCE(remote_uid,'')
		FROM events
		WHERE calendar_id = ?
		ORDER BY start_time ASC`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventsSimple(rows)
}

func (s *Store) GetEventsByCalendarIDRange(ctx context.Context, calendarID string, start, end time.Time) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, description, start_time, end_time, all_day, location, created_at, updated_at,
			COALESCE(account_id,''), COALESCE(calendar_id,''), COALESCE(remote_uid,'')
		FROM events
		WHERE calendar_id = ? AND start_time <= ? AND end_time >= ?
		ORDER BY start_time ASC`, calendarID, end.Unix(), start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventsSimple(rows)
}

func (s *Store) DeleteEventByRemoteUID(ctx context.Context, remoteUID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE remote_uid = ?", remoteUID)
	return err
}

func (s *Store) GetCalendarByPath(ctx context.Context, path string) (*Calendar, error) {
	var c Calendar
	var visible int
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, account_id, name, href, color, is_visible, created_at, updated_at FROM calendars WHERE id = ? OR href = ?", path, path).
		Scan(&c.ID, &c.AccountID, &c.Name, &c.Href, &c.Color, &visible, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.IsVisible = visible == 1
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	return &c, nil
}

func scanEventsSimple(rows *sql.Rows) ([]*Event, error) {
	out := make([]*Event, 0)
	for rows.Next() {
		var e Event
		var startTime, endTime, createdAt, updatedAt int64
		var allDay int
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &startTime, &endTime, &allDay, &e.Location, &createdAt, &updatedAt, &e.AccountID, &e.CalendarID, &e.RemoteUID); err != nil {
			return nil, err
		}
		e.StartTime = time.Unix(startTime, 0)
		e.EndTime = time.Unix(endTime, 0)
		e.AllDay = allDay == 1
		e.CreatedAt = time.Unix(createdAt, 0)
		e.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &e)
	}
	return out, nil
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
			COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(c.color, a.color, '#7d6b96'), COALESCE(e.recurrence,''), COALESCE(e.timezone,'')
		FROM events e
		LEFT JOIN calendars c ON e.calendar_id = c.id
		LEFT JOIN accounts a ON e.account_id = a.id
		JOIN events_fts ON e.rowid = events_fts.rowid
		WHERE events_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]*Event, error) {
	out := make([]*Event, 0)
	for rows.Next() {
		var e Event
		var startTime, endTime, createdAt, updatedAt int64
		var allDay int
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &startTime, &endTime, &allDay, &e.Location, &createdAt, &updatedAt, &e.AccountID, &e.CalendarID, &e.Color, &e.Recurrence, &e.Timezone); err != nil {
			return nil, err
		}
		e.StartTime = time.Unix(startTime, 0)
		e.EndTime = time.Unix(endTime, 0)
		e.AllDay = allDay == 1
		e.CreatedAt = time.Unix(createdAt, 0)
		e.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &e)
	}
	return out, nil
}

func expandRecurringEvents(events []*Event, rangeStart, rangeEnd time.Time) []*Event {
	out := make([]*Event, 0, len(events))
	for _, e := range events {
		if e.Recurrence == "" {
			out = append(out, e)
			continue
		}
		roption, err := rrule.StrToROption(e.Recurrence)
		if err != nil {
			out = append(out, e)
			continue
		}
		roption.Dtstart = e.StartTime
		rule, err := rrule.NewRRule(*roption)
		if err != nil {
			out = append(out, e)
			continue
		}
		duration := e.EndTime.Sub(e.StartTime)
		occurrences := rule.Between(rangeStart.Add(-duration), rangeEnd, true)
		for _, occ := range occurrences {
			expanded := &Event{
				ID:          e.ID,
				AccountID:   e.AccountID,
				CalendarID:  e.CalendarID,
				RemoteUID:   e.RemoteUID,
				Title:       e.Title,
				Description: e.Description,
				StartTime:   occ,
				EndTime:     occ.Add(duration),
				AllDay:      e.AllDay,
				Location:    e.Location,
				Recurrence:  e.Recurrence,
				Color:       e.Color,
				CreatedAt:   e.CreatedAt,
				UpdatedAt:   e.UpdatedAt,
			}
			out = append(out, expanded)
		}
	}
	return out
}

func (s *Store) GetConflictingEvents(ctx context.Context, start, end time.Time, excludeID string) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
			COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(c.color, a.color, '#7d6b96'), COALESCE(e.recurrence,''), COALESCE(e.timezone,'')
		FROM events e
		LEFT JOIN calendars c ON e.calendar_id = c.id
		LEFT JOIN accounts a ON e.account_id = a.id
		WHERE e.start_time < ? AND e.end_time > ? AND e.id != ? AND e.all_day = 0
		ORDER BY e.start_time ASC`,
		end.Unix(), start.Unix(), excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) AddReminder(ctx context.Context, eventID string, minutesBefore int) (*Reminder, error) {
	id := newEventID()
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO reminders (id, event_id, minutes_before, notified, created_at) VALUES (?, ?, ?, 0, ?)",
		id, eventID, minutesBefore, now)
	if err != nil {
		return nil, err
	}
	return &Reminder{ID: id, EventID: eventID, MinutesBefore: minutesBefore, CreatedAt: now}, nil
}

func (s *Store) ListReminders(ctx context.Context, eventID string) ([]*Reminder, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, event_id, minutes_before, notified, created_at FROM reminders WHERE event_id = ? ORDER BY minutes_before ASC", eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Reminder, 0)
	for rows.Next() {
		var r Reminder
		var notified int
		if err := rows.Scan(&r.ID, &r.EventID, &r.MinutesBefore, &notified, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Notified = notified == 1
		out = append(out, &r)
	}
	return out, nil
}

func (s *Store) DeleteReminder(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM reminders WHERE id = ?", id)
	return err
}

func (s *Store) GetPendingReminders(ctx context.Context) ([]*PendingReminder, error) {
	now := time.Now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, e.title, e.start_time, r.minutes_before, e.location
		FROM reminders r
		JOIN events e ON r.event_id = e.id
		WHERE r.notified = 0
			AND e.start_time - (r.minutes_before * 60) <= ?
			AND e.start_time >= ?`,
		now, now-(3600))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*PendingReminder, 0)
	for rows.Next() {
		var p PendingReminder
		var startUnix int64
		if err := rows.Scan(&p.ReminderID, &p.EventTitle, &startUnix, &p.MinutesBefore, &p.Location); err != nil {
			return nil, err
		}
		p.EventStart = time.Unix(startUnix, 0).Format(time.RFC3339)
		out = append(out, &p)
	}
	return out, nil
}

func (s *Store) MarkReminderNotified(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE reminders SET notified = 1 WHERE id = ?", id)
	return err
}

func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func newEventUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d@localitas", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b[:]) + "@localitas"
}
