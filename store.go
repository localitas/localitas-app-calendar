package calendar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/localitas/localitas-go"
	"github.com/teambition/rrule-go"
)

const DatabaseName = "calendar"

// ErrNotFound is returned for a record that does not exist or belongs to
// another user: the two are indistinguishable to the caller.
var ErrNotFound = errors.New("not found")

// Every calendar belongs to an account and every account to a user, so a
// user's records are those reached through calendars of their accounts.
const ownedCalendarIDs = `SELECT c.id FROM calendars c JOIN accounts a ON a.id = c.account_id WHERE a.user_id = ?`

const eventColumns = `e.id, e.title, e.description, e.start_time, e.end_time, e.all_day, e.location, e.created_at, e.updated_at,
	COALESCE(e.account_id,''), COALESCE(e.calendar_id,''), COALESCE(e.remote_uid,''), COALESCE(c.color, a.color, '#7d6b96'), COALESCE(e.recurrence,''), COALESCE(e.timezone,'')`

const ownedEventsFrom = `FROM events e
	JOIN calendars c ON c.id = e.calendar_id
	JOIN accounts a ON a.id = c.account_id
	WHERE a.user_id = ?`

const calendarColumns = `c.id, c.account_id, c.name, c.href, c.color, c.is_visible, c.created_at, c.updated_at`

const accountColumns = `id, user_id, name, provider, email, caldav_url, username, color, COALESCE(vault_credential_id,''), is_active, last_synced_at, COALESCE(sync_error,''), created_at, updated_at`

// localProvider marks the account holding a user's local calendar: events
// not pushed to any remote calendar. It is created on demand and never
// shown, edited or synced as an account.
const localProvider = "local"

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

func requireOneRow(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func localCalendarID(userID string) string {
	return "local-" + userID
}

func (s *Store) CreateAccount(ctx context.Context, userID, name, provider, email, caldavURL, username, password, oauthClientID, oauthClientSecret, color, vaultCredentialID string) (*CalendarAccount, error) {
	if provider == localProvider {
		return nil, fmt.Errorf("provider %q is reserved", localProvider)
	}
	id := newEventID()
	now := time.Now().UTC().Unix()
	if color == "" {
		color = "#7d6b96"
	}
	encPass, err := client.Encrypt(password)
	if err != nil {
		return nil, fmt.Errorf("encrypt password: %w", err)
	}
	encSecret, err := client.Encrypt(oauthClientSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt oauth_client_secret: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO accounts (id, user_id, name, provider, email, caldav_url, username, password, oauth_client_id, oauth_client_secret, color, vault_credential_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, userID, name, provider, email, caldavURL, username, encPass, oauthClientID, encSecret, color, vaultCredentialID, now, now)
	if err != nil {
		return nil, err
	}
	return &CalendarAccount{ID: id, UserID: userID, Name: name, Provider: provider, Email: email, CalDAVURL: caldavURL, Username: username, OAuthClientID: oauthClientID, VaultCredentialID: vaultCredentialID, Color: color, IsActive: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func scanAccount(scan func(...any) error) (*CalendarAccount, error) {
	var a CalendarAccount
	var active int
	var lastSynced *int64
	var createdAt, updatedAt int64
	if err := scan(&a.ID, &a.UserID, &a.Name, &a.Provider, &a.Email, &a.CalDAVURL, &a.Username, &a.Color, &a.VaultCredentialID, &active, &lastSynced, &a.SyncError, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	a.IsActive = active == 1
	a.LastSyncedAt = lastSynced
	a.CreatedAt = time.Unix(createdAt, 0)
	a.UpdatedAt = time.Unix(updatedAt, 0)
	return &a, nil
}

func (s *Store) queryAccounts(ctx context.Context, query string, args ...any) ([]*CalendarAccount, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*CalendarAccount, 0)
	for rows.Next() {
		a, err := scanAccount(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAccounts returns userID's accounts.
func (s *Store) ListAccounts(ctx context.Context, userID string) ([]*CalendarAccount, error) {
	return s.queryAccounts(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = ? AND provider != ? ORDER BY name`, userID, localProvider)
}

// ListAllAccounts returns every user's accounts, for the system sync job.
func (s *Store) ListAllAccounts(ctx context.Context) ([]*CalendarAccount, error) {
	return s.queryAccounts(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE provider != ? ORDER BY user_id, name`, localProvider)
}

// GetAccount returns userID's account id with its secrets decrypted.
func (s *Store) GetAccount(ctx context.Context, userID, id string) (*CalendarAccount, error) {
	var password, clientID, secret string
	row := s.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+`, password, COALESCE(oauth_client_id,''), COALESCE(oauth_client_secret,'') FROM accounts WHERE id = ? AND user_id = ? AND provider != ?`,
		id, userID, localProvider)
	a, err := scanAccount(func(dest ...any) error {
		return row.Scan(append(dest, &password, &clientID, &secret)...)
	})
	if err != nil {
		return nil, notFound(err)
	}
	a.OAuthClientID = clientID
	if a.Password, err = client.Decrypt(password); err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}
	if a.OAuthClientSecret, err = client.Decrypt(secret); err != nil {
		return nil, fmt.Errorf("decrypt oauth_client_secret: %w", err)
	}
	return a, nil
}

// UpdateAccount rewrites userID's account id.
func (s *Store) UpdateAccount(ctx context.Context, userID, id, name, provider, email, caldavURL, username, password, color, vaultCredentialID string) error {
	if provider == localProvider {
		return fmt.Errorf("provider %q is reserved", localProvider)
	}
	now := time.Now().UTC().Unix()
	if err := requireOneRow(s.db.ExecContext(ctx,
		`UPDATE accounts SET name=?, provider=?, email=?, caldav_url=?, username=?, color=?, vault_credential_id=?, updated_at=? WHERE id=? AND user_id=? AND provider != ?`,
		name, provider, email, caldavURL, username, color, vaultCredentialID, now, id, userID, localProvider)); err != nil {
		return err
	}
	// An update that omits the password keeps the stored one; writing
	// Encrypt("") would silently blank it.
	if password == "" {
		return nil
	}
	encPass, err := client.Encrypt(password)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}
	return requireOneRow(s.db.ExecContext(ctx, `UPDATE accounts SET password=? WHERE id=? AND user_id=?`, encPass, id, userID))
}

// DeleteAccount removes userID's account id with its calendars, events and
// reminders.
func (s *Store) DeleteAccount(ctx context.Context, userID, id string) error {
	if err := requireOneRow(s.db.ExecContext(ctx,
		"DELETE FROM accounts WHERE id = ? AND user_id = ? AND provider != ?", id, userID, localProvider)); err != nil {
		return err
	}
	accountEvents := `SELECT id FROM events WHERE account_id = ? OR calendar_id IN (SELECT id FROM calendars WHERE account_id = ?)`
	if _, err := s.db.ExecContext(ctx, "DELETE FROM reminders WHERE event_id IN ("+accountEvents+")", id, id); err != nil {
		return fmt.Errorf("delete reminders: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id IN ("+accountEvents+")", id, id); err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM calendars WHERE account_id = ?", id); err != nil {
		return fmt.Errorf("delete calendars: %w", err)
	}
	return nil
}

// UpdateSyncStatus records the outcome of syncing userID's account id.
func (s *Store) UpdateSyncStatus(ctx context.Context, userID, id, syncErr string) error {
	now := time.Now().UTC().Unix()
	return requireOneRow(s.db.ExecContext(ctx,
		"UPDATE accounts SET last_synced_at = ?, sync_error = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		now, syncErr, now, id, userID))
}

// EnsureLocalCalendar returns userID's local calendar, creating it and its
// account on first use.
func (s *Store) EnsureLocalCalendar(ctx context.Context, userID string) (*Calendar, error) {
	if userID == "" {
		return nil, fmt.Errorf("local calendar needs a user")
	}
	id := localCalendarID(userID)
	now := time.Now().UTC().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO accounts (id, user_id, name, provider, color, is_active, created_at, updated_at) VALUES (?, ?, 'Local', ?, '#7d6b96', 1, ?, ?)`,
		id, userID, localProvider, now, now); err != nil {
		return nil, fmt.Errorf("create local account: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO calendars (id, account_id, name, href, color, is_visible, created_at, updated_at) VALUES (?, ?, 'Local', '', '#7d6b96', 1, ?, ?)`,
		id, id, now, now); err != nil {
		return nil, fmt.Errorf("create local calendar: %w", err)
	}
	return s.GetCalendar(ctx, userID, id)
}

// UpsertCalendar records a calendar of accountID, which the caller has
// already established belongs to the acting user.
func (s *Store) UpsertCalendar(ctx context.Context, accountID, name, href, color string) (*Calendar, error) {
	var existing string
	var existingColor string
	err := s.db.QueryRowContext(ctx, "SELECT id, color FROM calendars WHERE account_id = ? AND (href = ? OR name = ?)", accountID, href, name).Scan(&existing, &existingColor)
	now := time.Now().UTC().Unix()
	switch {
	case err == nil:
		if _, err := s.db.ExecContext(ctx, "UPDATE calendars SET name = ?, href = ?, updated_at = ? WHERE id = ?", name, href, now, existing); err != nil {
			return nil, fmt.Errorf("update calendar: %w", err)
		}
		return &Calendar{ID: existing, AccountID: accountID, Name: name, Href: href, Color: existingColor, IsVisible: true}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("find calendar: %w", err)
	}
	id := newEventID()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO calendars (id, account_id, name, href, color, is_visible, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
		id, accountID, name, href, color, now, now); err != nil {
		return nil, err
	}
	return &Calendar{ID: id, AccountID: accountID, Name: name, Href: href, Color: color, IsVisible: true, CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0)}, nil
}

func scanCalendar(scan func(...any) error) (*Calendar, error) {
	var c Calendar
	var visible int
	var createdAt, updatedAt int64
	if err := scan(&c.ID, &c.AccountID, &c.Name, &c.Href, &c.Color, &visible, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.IsVisible = visible == 1
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)
	return &c, nil
}

// ListCalendars returns userID's calendars, only those of accountID when it
// is set.
func (s *Store) ListCalendars(ctx context.Context, userID, accountID string) ([]*Calendar, error) {
	query := `SELECT ` + calendarColumns + ` FROM calendars c JOIN accounts a ON a.id = c.account_id WHERE a.user_id = ?`
	args := []any{userID}
	if accountID != "" {
		query += ` AND c.account_id = ?`
		args = append(args, accountID)
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY c.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Calendar, 0)
	for rows.Next() {
		c, err := scanCalendar(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCalendar returns userID's calendar id.
func (s *Store) GetCalendar(ctx context.Context, userID, id string) (*Calendar, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+calendarColumns+` FROM calendars c JOIN accounts a ON a.id = c.account_id WHERE c.id = ? AND a.user_id = ?`, id, userID)
	c, err := scanCalendar(row.Scan)
	if err != nil {
		return nil, notFound(err)
	}
	return c, nil
}

// GetCalendarByPath returns userID's calendar named by a CalDAV path
// segment: its ID, or the /caldav/ path it was created under.
func (s *Store) GetCalendarByPath(ctx context.Context, userID, segment string) (*Calendar, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+calendarColumns+` FROM calendars c JOIN accounts a ON a.id = c.account_id WHERE (c.id = ? OR c.href = ?) AND a.user_id = ? ORDER BY c.id = ? DESC LIMIT 1`,
		segment, calendarPath(segment), userID, segment)
	c, err := scanCalendar(row.Scan)
	if err != nil {
		return nil, notFound(err)
	}
	return c, nil
}

// UpdateCalendar changes the color (when set) and visibility (when non-nil)
// of userID's calendar id.
func (s *Store) UpdateCalendar(ctx context.Context, userID, id, color string, visible *bool) error {
	if _, err := s.GetCalendar(ctx, userID, id); err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	if color != "" {
		if err := requireOneRow(s.db.ExecContext(ctx,
			"UPDATE calendars SET color = ?, updated_at = ? WHERE id = ? AND id IN ("+ownedCalendarIDs+")", color, now, id, userID)); err != nil {
			return err
		}
	}
	if visible != nil {
		v := 0
		if *visible {
			v = 1
		}
		if err := requireOneRow(s.db.ExecContext(ctx,
			"UPDATE calendars SET is_visible = ?, updated_at = ? WHERE id = ? AND id IN ("+ownedCalendarIDs+")", v, now, id, userID)); err != nil {
			return err
		}
	}
	return nil
}

// Create adds an event to cal, which the caller has already established
// belongs to the acting user.
func (s *Store) Create(ctx context.Context, cal *Calendar, remoteUID, title, description, location, timezone string, startTime, endTime time.Time, allDay bool) (*Event, error) {
	id := newEventID()
	now := time.Now().UTC().Unix()
	allDayInt := 0
	if allDay {
		allDayInt = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, calendar_id, account_id, remote_uid, title, description, start_time, end_time, all_day, location, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, cal.ID, cal.AccountID, remoteUID, title, description, startTime.Unix(), endTime.Unix(), allDayInt, location, timezone, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}

	return &Event{
		ID: id, CalendarID: cal.ID, AccountID: cal.AccountID, RemoteUID: remoteUID,
		Title: title, Description: description,
		StartTime: startTime, EndTime: endTime, AllDay: allDay,
		Location: location, Timezone: timezone, Color: cal.Color,
		CreatedAt: time.Unix(now, 0), UpdatedAt: time.Unix(now, 0),
	}, nil
}

func (s *Store) queryEvent(ctx context.Context, where string, args ...any) (*Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` `+ownedEventsFrom+` AND `+where+` LIMIT 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrNotFound
	}
	return events[0], nil
}

func (s *Store) queryEvents(ctx context.Context, where string, args ...any) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` `+ownedEventsFrom+` AND `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// Get returns userID's event id.
func (s *Store) Get(ctx context.Context, userID, id string) (*Event, error) {
	return s.queryEvent(ctx, `e.id = ?`, userID, id)
}

// GetEventsByRange returns userID's events on visible calendars that
// overlap start..end, with recurring events expanded.
func (s *Store) GetEventsByRange(ctx context.Context, userID string, start, end time.Time) ([]*Event, error) {
	events, err := s.queryEvents(ctx,
		`((e.start_time <= ? AND e.end_time >= ?) OR e.recurrence != '') AND c.is_visible = 1 ORDER BY e.start_time ASC`,
		userID, end.Unix(), start.Unix())
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	return expandRecurringEvents(events, start, end), nil
}

func (s *Store) GetEventsByDay(ctx context.Context, userID string, start, end time.Time) ([]DayEvents, error) {
	events, err := s.GetEventsByRange(ctx, userID, start, end)
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

// Update rewrites userID's event id.
func (s *Store) Update(ctx context.Context, userID, id, title, description, location string, startTime, endTime time.Time, allDay bool) error {
	now := time.Now().UTC().Unix()
	allDayInt := 0
	if allDay {
		allDayInt = 1
	}
	return requireOneRow(s.db.ExecContext(ctx, `
		UPDATE events SET title=?, description=?, start_time=?, end_time=?, all_day=?, location=?, updated_at=?
		WHERE id=? AND calendar_id IN (`+ownedCalendarIDs+`)`,
		title, description, startTime.Unix(), endTime.Unix(), allDayInt, location, now, id, userID))
}

// Delete removes userID's event id and its reminders.
func (s *Store) Delete(ctx context.Context, userID, id string) error {
	if err := requireOneRow(s.db.ExecContext(ctx,
		"DELETE FROM events WHERE id = ? AND calendar_id IN ("+ownedCalendarIDs+")", id, userID)); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM reminders WHERE event_id = ?", id); err != nil {
		return fmt.Errorf("delete reminders: %w", err)
	}
	return nil
}

// GetEventByRemoteUID returns the event with remoteUID in userID's calendar
// calID.
func (s *Store) GetEventByRemoteUID(ctx context.Context, userID, calID, remoteUID string) (*Event, error) {
	return s.queryEvent(ctx, `e.calendar_id = ? AND e.remote_uid = ?`, userID, calID, remoteUID)
}

// GetEventsByCalendarID returns the events of userID's calendar calID.
func (s *Store) GetEventsByCalendarID(ctx context.Context, userID, calID string) ([]*Event, error) {
	return s.queryEvents(ctx, `e.calendar_id = ? ORDER BY e.start_time ASC`, userID, calID)
}

// GetEventsByCalendarIDRange returns the events of userID's calendar calID
// overlapping start..end.
func (s *Store) GetEventsByCalendarIDRange(ctx context.Context, userID, calID string, start, end time.Time) ([]*Event, error) {
	return s.queryEvents(ctx, `e.calendar_id = ? AND e.start_time <= ? AND e.end_time >= ? ORDER BY e.start_time ASC`,
		userID, calID, end.Unix(), start.Unix())
}

// DeleteEventByRemoteUID removes the event with remoteUID from userID's
// calendar calID.
func (s *Store) DeleteEventByRemoteUID(ctx context.Context, userID, calID, remoteUID string) error {
	e, err := s.GetEventByRemoteUID(ctx, userID, calID, remoteUID)
	if err != nil {
		return err
	}
	return s.Delete(ctx, userID, e.ID)
}

// Search returns userID's events matching query.
func (s *Store) Search(ctx context.Context, userID, query string, limit int) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+eventColumns+`
		FROM events e
		JOIN calendars c ON c.id = e.calendar_id
		JOIN accounts a ON a.id = c.account_id
		JOIN events_fts ON e.rowid = events_fts.rowid
		WHERE a.user_id = ? AND events_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, userID, query, limit)
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
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &startTime, &endTime, &allDay, &e.Location, &createdAt, &updatedAt, &e.AccountID, &e.CalendarID, &e.RemoteUID, &e.Color, &e.Recurrence, &e.Timezone); err != nil {
			return nil, err
		}
		e.StartTime = time.Unix(startTime, 0)
		e.EndTime = time.Unix(endTime, 0)
		e.AllDay = allDay == 1
		e.CreatedAt = time.Unix(createdAt, 0)
		e.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, &e)
	}
	return out, rows.Err()
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

// GetConflictingEvents returns userID's timed events overlapping start..end.
func (s *Store) GetConflictingEvents(ctx context.Context, userID string, start, end time.Time, excludeID string) ([]*Event, error) {
	return s.queryEvents(ctx,
		`e.start_time < ? AND e.end_time > ? AND e.id != ? AND e.all_day = 0 ORDER BY e.start_time ASC`,
		userID, end.Unix(), start.Unix(), excludeID)
}

// AddReminder adds a reminder to userID's event eventID.
func (s *Store) AddReminder(ctx context.Context, userID, eventID string, minutesBefore int) (*Reminder, error) {
	if _, err := s.Get(ctx, userID, eventID); err != nil {
		return nil, err
	}
	return s.insertReminder(ctx, eventID, minutesBefore)
}

func (s *Store) insertReminder(ctx context.Context, eventID string, minutesBefore int) (*Reminder, error) {
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

// SetReminders replaces the reminders of userID's event eventID; minutes
// that are not positive are skipped.
func (s *Store) SetReminders(ctx context.Context, userID, eventID string, minutesBefore []int) ([]*Reminder, error) {
	if _, err := s.Get(ctx, userID, eventID); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM reminders WHERE event_id = ?", eventID); err != nil {
		return nil, fmt.Errorf("clear reminders: %w", err)
	}
	out := make([]*Reminder, 0, len(minutesBefore))
	for _, mins := range minutesBefore {
		if mins <= 0 {
			continue
		}
		rem, err := s.insertReminder(ctx, eventID, mins)
		if err != nil {
			return nil, err
		}
		out = append(out, rem)
	}
	return out, nil
}

// ListReminders returns the reminders of userID's event eventID.
func (s *Store) ListReminders(ctx context.Context, userID, eventID string) ([]*Reminder, error) {
	if _, err := s.Get(ctx, userID, eventID); err != nil {
		return nil, err
	}
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
	return out, rows.Err()
}

// DeleteReminder removes reminder id from one of userID's events.
func (s *Store) DeleteReminder(ctx context.Context, userID, id string) error {
	return requireOneRow(s.db.ExecContext(ctx,
		"DELETE FROM reminders WHERE id = ? AND event_id IN (SELECT id FROM events WHERE calendar_id IN ("+ownedCalendarIDs+"))", id, userID))
}

// GetPendingReminders returns userID's due, unnotified reminders.
func (s *Store) GetPendingReminders(ctx context.Context, userID string) ([]*PendingReminder, error) {
	return s.pendingReminders(ctx, "AND a.user_id = ?", userID)
}

// GetAllPendingReminders returns every user's due, unnotified reminders,
// for the system reminder job.
func (s *Store) GetAllPendingReminders(ctx context.Context) ([]*PendingReminder, error) {
	return s.pendingReminders(ctx, "")
}

func (s *Store) pendingReminders(ctx context.Context, filter string, args ...any) ([]*PendingReminder, error) {
	now := time.Now().UTC().Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, e.title, e.start_time, r.minutes_before, e.location, a.user_id
		FROM reminders r
		JOIN events e ON r.event_id = e.id
		JOIN calendars c ON c.id = e.calendar_id
		JOIN accounts a ON a.id = c.account_id
		WHERE r.notified = 0
			AND e.start_time - (r.minutes_before * 60) <= ?
			AND e.start_time >= ? `+filter,
		append([]any{now, now - 3600}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*PendingReminder, 0)
	for rows.Next() {
		var p PendingReminder
		var startUnix int64
		if err := rows.Scan(&p.ReminderID, &p.EventTitle, &startUnix, &p.MinutesBefore, &p.Location, &p.UserID); err != nil {
			return nil, err
		}
		p.EventStart = time.Unix(startUnix, 0).Format(time.RFC3339)
		out = append(out, &p)
	}
	return out, rows.Err()
}

// MarkReminderNotified marks reminder id notified, for the system reminder
// job.
func (s *Store) MarkReminderNotified(ctx context.Context, id string) error {
	return requireOneRow(s.db.ExecContext(ctx, "UPDATE reminders SET notified = 1 WHERE id = ?", id))
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
