package calendar

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
)

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
			log.Printf("install: attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := applyEmbeddedMigrations(ctx, a.client, db.ID); err != nil {
			log.Printf("install: migrations attempt %d failed (retrying): %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		return db.ID, nil
	}
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/index.html")
	if err != nil {
		log.Printf("calendar index template error: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	data := map[string]string{"BasePath": a.BasePath}
	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("calendar index render error: %v", err)
	}
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	h := &handler{app: a}

	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /swagger.json", HandleSwagger)
	mux.HandleFunc("GET /help.md", handleHelpMarkdown)
	mux.HandleFunc("GET /api/events", h.handleGetEvents)
	mux.HandleFunc("POST /api/events", h.handleCreateEvent)
	mux.HandleFunc("GET /api/events/{id}", h.handleGetEvent)
	mux.HandleFunc("PUT /api/events/{id}", h.handleUpdateEvent)
	mux.HandleFunc("DELETE /api/events/{id}", h.handleDeleteEvent)
	mux.HandleFunc("GET /api/search", h.handleSearch)
	mux.HandleFunc("GET /api/parse-date", h.handleParseDate)
	mux.HandleFunc("GET /api/accounts", h.handleListAccounts)
	mux.HandleFunc("POST /api/accounts", h.handleCreateAccount)
	mux.HandleFunc("PUT /api/accounts/{id}", h.handleUpdateAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", h.handleDeleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/sync", h.handleSyncAccount)
	mux.HandleFunc("POST /api/sync", h.handleSyncAll)
	mux.HandleFunc("GET /api/oauth/{id}/start", h.handleOAuthStart)
	mux.HandleFunc("GET /api/oauth/callback", h.handleOAuthCallback)
	mux.HandleFunc("GET /api/calendars", h.handleListCalendars)
	mux.HandleFunc("PUT /api/calendars/{id}", h.handleUpdateCalendar)
	mux.HandleFunc("POST /api/events/{id}/reminders", h.handleAddReminder)
	mux.HandleFunc("PUT /api/events/{id}/reminders", h.handleSetReminders)
	mux.HandleFunc("GET /api/events/{id}/reminders", h.handleListReminders)
	mux.HandleFunc("DELETE /api/reminders/{rid}", h.handleDeleteReminder)
	mux.HandleFunc("GET /api/reminders/pending", h.handlePendingReminders)
	mux.HandleFunc("POST /api/reminders/check", h.handleCheckReminders)
	mux.HandleFunc("GET /api/conflicts", h.handleCheckConflicts)
	mux.HandleFunc("GET /api/presets", h.handleListPresets)

	caldavHandler := NewCalDAVHandler(a.Store, "/caldav/")
	mux.Handle("/caldav/", caldavHandler)
	mux.HandleFunc("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, a.BasePath+"caldav/", http.StatusPermanentRedirect)
	})
}
