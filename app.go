package calendar

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

var logger = slog.Default().With("component", "calendar")

type App struct {
	Store          *Store
	CalDAV         *CalDAVBackend
	BasePath       string
	client         *client.Client
	GoogleClientID string
	GoogleSecret   string
}

func (a *App) SetGoogleOAuth(clientID, secret string) {
	a.GoogleClientID = clientID
	a.GoogleSecret = secret
}

func New(c *client.Client, basePath string) *App {
	if basePath == "" {
		basePath = "/"
	}
	return &App{
		BasePath: basePath,
		client:   c,
	}
}

func (a *App) InitStore(coreURL, dbID, token string) error {
	store, err := OpenStore(coreURL, dbID, token)
	if err != nil {
		return err
	}
	a.Store = store
	a.CalDAV = NewCalDAVBackend(store)
	return nil
}

func (a *App) Install(ctx context.Context) (string, error) {
	for attempt := 1; ; attempt++ {
		db, err := a.client.CreateSystemDatabase(ctx, DatabaseName)
		if err != nil {
			logger.Warn("install attempt failed", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			logger.Warn("install migrations attempt failed", "attempt", attempt, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		logger.Error("index template error", "error", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	data := map[string]string{"BasePath": a.BasePath}
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		logger.Error("index render error", "error", err)
	}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	h := &handler{app: a}

	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /swagger.json", HandleSwagger)
	mux.HandleFunc("GET /help.md", handleHelpMarkdown)

	// Every user route acts for the user core vouches for; one without a
	// verified user is refused, and changing anything needs write scope.
	user := func(pattern string, handler http.HandlerFunc) {
		mux.Handle(pattern, client.AuthMiddleware(handler))
	}
	writer := func(pattern string, handler http.HandlerFunc) {
		user(pattern, client.RequireScopeFunc(client.ScopeWrite, handler))
	}
	// Cron jobs act across every user's data, so only core may call them.
	system := func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, client.RequireSystem(handler))
	}

	user("GET /api/events", h.handleGetEvents)
	writer("POST /api/events", h.handleCreateEvent)
	user("GET /api/events/{id}", h.handleGetEvent)
	writer("PUT /api/events/{id}", h.handleUpdateEvent)
	writer("DELETE /api/events/{id}", h.handleDeleteEvent)
	user("GET /api/search", h.handleSearch)
	user("GET /api/parse-date", h.handleParseDate)
	user("GET /api/accounts", h.handleListAccounts)
	writer("POST /api/accounts", h.handleCreateAccount)
	writer("PUT /api/accounts/{id}", h.handleUpdateAccount)
	writer("DELETE /api/accounts/{id}", h.handleDeleteAccount)
	writer("POST /api/accounts/{id}/sync", h.handleSyncAccount)
	writer("POST /api/accounts/sync", h.handleSyncMine)
	writer("GET /api/oauth/{id}/start", h.handleOAuthStart)
	writer("GET /api/oauth/callback", h.handleOAuthCallback)
	user("GET /api/calendars", h.handleListCalendars)
	writer("PUT /api/calendars/{id}", h.handleUpdateCalendar)
	writer("POST /api/events/{id}/reminders", h.handleAddReminder)
	writer("PUT /api/events/{id}/reminders", h.handleSetReminders)
	user("GET /api/events/{id}/reminders", h.handleListReminders)
	writer("DELETE /api/reminders/{rid}", h.handleDeleteReminder)
	user("GET /api/reminders/pending", h.handlePendingReminders)
	user("GET /api/conflicts", h.handleCheckConflicts)
	user("GET /api/presets", h.handleListPresets)

	system("POST /api/sync", h.handleSyncAll)
	system("POST /api/reminders/check", h.handleCheckReminders)

	mux.Handle("/caldav/", NewCalDAVHandler(a.Store, caldavPrefix))
	mux.HandleFunc("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, a.BasePath+"caldav/", http.StatusPermanentRedirect)
	})
}
