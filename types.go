package calendar

import "time"

type CalendarAccount struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Provider          string    `json:"provider"`
	Email             string    `json:"email"`
	CalDAVURL         string    `json:"caldav_url"`
	Username          string    `json:"username"`
	Password          string    `json:"-"`
	OAuthClientID     string    `json:"oauth_client_id,omitempty"`
	OAuthClientSecret string    `json:"-"`
	AccessToken       string    `json:"-"`
	RefreshToken      string    `json:"-"`
	TokenExpiry       int64     `json:"token_expiry,omitempty"`
	VaultCredentialID string    `json:"vault_credential_id,omitempty"`
	Color             string    `json:"color"`
	IsActive          bool      `json:"is_active"`
	LastSyncedAt      *int64    `json:"last_synced_at,omitempty"`
	SyncError         string    `json:"sync_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (a *CalendarAccount) NeedsOAuth() bool {
	return a.Provider == "google"
}

func (a *CalendarAccount) HasValidToken() bool {
	return a.AccessToken != "" && a.TokenExpiry > time.Now().UTC().Unix()
}

type Calendar struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id"`
	Name      string    `json:"name"`
	Href      string    `json:"href"`
	Color     string    `json:"color"`
	IsVisible bool      `json:"is_visible"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Event struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id,omitempty"`
	CalendarID  string    `json:"calendar_id,omitempty"`
	RemoteUID   string    `json:"remote_uid,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	AllDay      bool      `json:"all_day"`
	Location    string    `json:"location"`
	Timezone    string    `json:"timezone,omitempty"`
	Recurrence  string    `json:"recurrence,omitempty"`
	Color       string    `json:"color,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Reminder struct {
	ID            string `json:"id"`
	EventID       string `json:"event_id"`
	MinutesBefore int    `json:"minutes_before"`
	Notified      bool   `json:"notified"`
	CreatedAt     int64  `json:"created_at"`
}

type PendingReminder struct {
	ReminderID    string `json:"reminder_id"`
	EventTitle    string `json:"event_title"`
	EventStart    string `json:"event_start"`
	MinutesBefore int    `json:"minutes_before"`
	Location      string `json:"location,omitempty"`
}

type DayEvents struct {
	Date      string   `json:"date"`
	DateLabel string   `json:"date_label"`
	Events    []*Event `json:"events"`
}

type ProviderPreset struct {
	Name      string `json:"name"`
	CalDAVURL string `json:"caldav_url"`
	Provider  string `json:"provider"`
}

var CalDAVPresets = []ProviderPreset{
	{Name: "Google Calendar", CalDAVURL: "https://apidata.googleusercontent.com/caldav/v2/", Provider: "google"},
	{Name: "Yahoo Calendar", CalDAVURL: "https://caldav.calendar.yahoo.com/dav/", Provider: "yahoo"},
	{Name: "iCloud", CalDAVURL: "https://caldav.icloud.com/", Provider: "icloud"},
	{Name: "Fastmail", CalDAVURL: "https://caldav.fastmail.com/dav/calendars/user/", Provider: "fastmail"},
	{Name: "CalDAV (Custom)", CalDAVURL: "", Provider: "caldav"},
}
