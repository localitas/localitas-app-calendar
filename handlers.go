package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
	"github.com/localitas/localitas-go/httputil"
)

type handler struct {
	app *App
}

func (h *handler) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var start, end time.Time
	var err error

	if startStr != "" {
		start, err = time.Parse("2006-01-02", startStr)
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "invalid start date")
			return
		}
	} else {
		start = time.Now().AddDate(0, 0, -7)
	}

	if endStr != "" {
		end, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "invalid end date")
			return
		}
		end = end.Add(24*time.Hour - time.Second)
	} else {
		end = time.Now().AddDate(0, 0, 14)
	}

	days, err := h.app.CalDAV.GetEventsByDay(r.Context(), client.GetUserID(r.Context()), start, end)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "failed to get events: %v", err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]interface{}{"days": days})
}

func (h *handler) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := h.app.CalDAV.GetEvent(r.Context(), client.GetUserID(r.Context()), id)
	if err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusOK, e)
}

func (h *handler) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		AllDay      bool   `json:"all_day"`
		Location    string `json:"location"`
		CalendarID  string `json:"calendar_id"`
		Timezone    string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeErr(w, r, http.StatusBadRequest, "title is required")
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		startTime, _ = time.Parse("2006-01-02", req.StartTime)
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		endTime, _ = time.Parse("2006-01-02", req.EndTime)
		if !endTime.IsZero() {
			endTime = endTime.Add(24*time.Hour - time.Second)
		}
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	if endTime.IsZero() || endTime.Before(startTime) {
		endTime = startTime.Add(time.Hour)
	}

	e, err := h.app.CalDAV.CreateEventJSON(r.Context(), client.GetUserID(r.Context()), req.CalendarID, req.Title, req.Description, req.Location, req.Timezone, startTime, endTime, req.AllDay)
	if err != nil {
		writeStoreErr(w, r, "calendar", err)
		return
	}
	writeJSON(w, r, http.StatusCreated, e)
}

func (h *handler) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		AllDay      bool   `json:"all_day"`
		Location    string `json:"location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	startTime, _ := time.Parse(time.RFC3339, req.StartTime)
	endTime, _ := time.Parse(time.RFC3339, req.EndTime)

	e, err := h.app.CalDAV.UpdateEventJSON(r.Context(), client.GetUserID(r.Context()), id, req.Title, req.Description, req.Location, startTime, endTime, req.AllDay)
	if err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusOK, e)
}

func (h *handler) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.app.CalDAV.DeleteEventJSON(r.Context(), client.GetUserID(r.Context()), id); err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeErr(w, r, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	events, err := h.app.CalDAV.SearchEvents(r.Context(), client.GetUserID(r.Context()), query, 20)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "search failed: %v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, events)
}

func (h *handler) handleParseDate(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeErr(w, r, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	parsed, ok := ParseNaturalDate(query)
	if !ok {
		writeJSON(w, r, http.StatusOK, map[string]interface{}{"parsed": false, "input": query})
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"parsed": true,
		"input":  query,
		"date":   parsed.Format("2006-01-02"),
		"label":  parsed.Format("Monday, January 2, 2006"),
	})
}

func (h *handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	userID := client.GetUserID(r.Context())
	accounts, err := h.app.Store.ListAccounts(r.Context(), userID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, accounts)
}

func (h *handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string `json:"name"`
		Provider          string `json:"provider"`
		Email             string `json:"email"`
		CalDAVURL         string `json:"caldav_url"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		OAuthClientID     string `json:"oauth_client_id"`
		OAuthClientSecret string `json:"oauth_client_secret"`
		Color             string `json:"color"`
		VaultCredentialID string `json:"vault_credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Provider == "" {
		req.Provider = "caldav"
	}

	var seededRefreshToken string
	if req.VaultCredentialID != "" && h.app.client != nil {
		secrets, err := h.app.client.WithToken(client.TokenFromRequest(r)).VaultGetSecrets(r.Context(), req.VaultCredentialID)
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "failed to resolve vault credential: %v", err)
			return
		}
		if req.Username == "" {
			req.Username = secrets["username"]
		}
		if req.Password == "" {
			req.Password = secrets["password"]
		}
		if req.CalDAVURL == "" {
			req.CalDAVURL = secrets["caldav_url"]
		}
		if req.OAuthClientID == "" {
			req.OAuthClientID = secrets["oauth_client_id"]
		}
		if req.OAuthClientSecret == "" {
			req.OAuthClientSecret = secrets["oauth_client_secret"]
		}
		if req.Email == "" {
			req.Email = secrets["email"]
		}
		// A vault credential may carry a pre-provisioned refresh_token (dev seed /
		// headless provisioning). Persist it below so the account is usable without
		// the interactive OAuth "Connect" flow after a data wipe.
		seededRefreshToken = secrets["refresh_token"]
	}

	if req.Provider == "google" {
		if req.Email == "" || req.OAuthClientID == "" || req.OAuthClientSecret == "" {
			writeErr(w, r, http.StatusBadRequest, "email, oauth_client_id, oauth_client_secret required for Google")
			return
		}
	} else {
		if req.CalDAVURL == "" || req.Username == "" || req.Password == "" {
			writeErr(w, r, http.StatusBadRequest, "caldav_url, username, password are required")
			return
		}
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	userID := client.GetUserID(r.Context())
	account, err := h.app.Store.CreateAccount(r.Context(), userID, req.Name, req.Provider, req.Email, req.CalDAVURL, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.Color, req.VaultCredentialID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	// Persist a pre-provisioned refresh token (from the vault credential) so the
	// account works immediately without the interactive OAuth flow. expiresIn=0
	// forces a refresh on first use, which mints a fresh access token.
	if seededRefreshToken != "" {
		if err := h.app.Store.SaveOAuthTokens(r.Context(), userID, account.ID, "", seededRefreshToken, 0); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "account created but failed to save seeded token: %v", err)
			return
		}
	}
	writeJSON(w, r, http.StatusCreated, account)
}

func (h *handler) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name              string `json:"name"`
		Provider          string `json:"provider"`
		Email             string `json:"email"`
		CalDAVURL         string `json:"caldav_url"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		Color             string `json:"color"`
		VaultCredentialID string `json:"vault_credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.app.Store.UpdateAccount(r.Context(), client.GetUserID(r.Context()), id, req.Name, req.Provider, req.Email, req.CalDAVURL, req.Username, req.Password, req.Color, req.VaultCredentialID); err != nil {
		writeStoreErr(w, r, "account", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.app.Store.DeleteAccount(r.Context(), client.GetUserID(r.Context()), id); err != nil {
		writeStoreErr(w, r, "account", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSyncAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.app.Store.GetAccount(r.Context(), client.GetUserID(r.Context()), r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, r, "account", err)
		return
	}
	newCount, err := h.syncAccount(r.Context(), account)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"new_events": newCount,
	})
}

// syncAccount syncs one loaded account and records the outcome on it.
func (h *handler) syncAccount(ctx context.Context, a *CalendarAccount) (int, error) {
	from := time.Now().UTC().AddDate(0, -1, 0)
	to := time.Now().UTC().AddDate(0, 3, 0)
	n, syncErr := SyncAccount(ctx, h.app.Store, a, from, to)
	status := ""
	if syncErr != nil {
		status = syncErr.Error()
	}
	if err := h.app.Store.UpdateSyncStatus(ctx, a.UserID, a.ID, status); err != nil {
		return n, errors.Join(syncErr, fmt.Errorf("record sync status: %w", err))
	}
	return n, syncErr
}

// syncAccounts syncs every active account in accounts, collecting the
// failures per account.
func (h *handler) syncAccounts(ctx context.Context, accounts []*CalendarAccount) map[string]interface{} {
	totalNew := 0
	syncErrors := make([]string, 0)
	for _, a := range accounts {
		if !a.IsActive {
			continue
		}
		n, err := h.syncAccount(ctx, a)
		totalNew += n
		if err != nil {
			syncErrors = append(syncErrors, a.Name+": "+err.Error())
		}
	}
	return map[string]interface{}{
		"accounts":   len(accounts),
		"new_events": totalNew,
		"errors":     syncErrors,
	}
}

// handleSyncMine syncs the caller's accounts.
func (h *handler) handleSyncMine(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.app.Store.ListAccounts(r.Context(), client.GetUserID(r.Context()))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, h.syncAccounts(r.Context(), accounts))
}

// handleSyncAll is the system sync job: it syncs every user's accounts.
func (h *handler) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	work := func(ctx context.Context) (map[string]interface{}, error) {
		accounts, err := h.app.Store.ListAllAccounts(ctx)
		if err != nil {
			return nil, err
		}
		return h.syncAccounts(ctx, accounts), nil
	}

	if h.app.client.RunAsync(w, r, work) {
		return
	}

	result, err := work(r.Context())
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *handler) oauthRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + h.app.BasePath + "api/oauth/callback"
}

func (h *handler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	account, err := h.app.Store.GetAccount(r.Context(), client.GetUserID(r.Context()), accountID)
	if err != nil {
		writeStoreErr(w, r, "account", err)
		return
	}
	if account.OAuthClientID == "" {
		writeErr(w, r, http.StatusBadRequest, "no OAuth client ID configured")
		return
	}
	redirectURI := h.oauthRedirectURI(r)
	authURL := GoogleAuthRedirectURL(account.OAuthClientID, redirectURI, accountID)
	writeJSON(w, r, http.StatusOK, map[string]string{"auth_url": authURL, "redirect_uri": redirectURI})
}

func (h *handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	accountID := r.URL.Query().Get("state")
	if code == "" || accountID == "" {
		writeErr(w, r, http.StatusBadRequest, "missing code or state")
		return
	}

	// Load WITH tokens: when Google omits refresh_token on re-consent, the
	// stored one is kept instead of being overwritten with "".
	// Only an account of the user completing the flow can take the tokens.
	userID := client.GetUserID(r.Context())
	account, err := h.app.Store.GetAccountWithTokens(r.Context(), userID, accountID)
	if err != nil {
		writeStoreErr(w, r, "account", err)
		return
	}

	redirectURI := h.oauthRedirectURI(r)
	tok, err := ExchangeGoogleCode(r.Context(), code, account.OAuthClientID, account.OAuthClientSecret, redirectURI)
	if err != nil {
		writeOAuthPage(w, "Authorization Failed", err.Error(), 3000)
		return
	}

	refreshToken := tok.RefreshToken
	if refreshToken == "" {
		refreshToken = account.RefreshToken
	}
	if err := h.app.Store.SaveOAuthTokens(r.Context(), userID, accountID, tok.AccessToken, refreshToken, tok.ExpiresIn); err != nil {
		writeOAuthPage(w, "Authorization Failed", "could not save tokens: "+err.Error(), 3000)
		return
	}
	writeOAuthPage(w, "Google Calendar Connected!", "You can close this window.", 2000)
}

func (h *handler) handleListCalendars(w http.ResponseWriter, r *http.Request) {
	calendars, err := h.app.Store.ListCalendars(r.Context(), client.GetUserID(r.Context()), r.URL.Query().Get("account_id"))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, calendars)
}

func (h *handler) handleUpdateCalendar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Color     string `json:"color"`
		IsVisible *bool  `json:"is_visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.app.Store.UpdateCalendar(r.Context(), client.GetUserID(r.Context()), r.PathValue("id"), req.Color, req.IsVisible); err != nil {
		writeStoreErr(w, r, "calendar", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleAddReminder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MinutesBefore int `json:"minutes_before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	if req.MinutesBefore <= 0 {
		req.MinutesBefore = 15
	}
	reminder, err := h.app.Store.AddReminder(r.Context(), client.GetUserID(r.Context()), r.PathValue("id"), req.MinutesBefore)
	if err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusCreated, reminder)
}

func (h *handler) handleListReminders(w http.ResponseWriter, r *http.Request) {
	reminders, err := h.app.Store.ListReminders(r.Context(), client.GetUserID(r.Context()), r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusOK, reminders)
}

func (h *handler) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Store.DeleteReminder(r.Context(), client.GetUserID(r.Context()), r.PathValue("rid")); err != nil {
		writeStoreErr(w, r, "reminder", err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handlePendingReminders(w http.ResponseWriter, r *http.Request) {
	pending, err := h.app.Store.GetPendingReminders(r.Context(), client.GetUserID(r.Context()))
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, pending)
}

// handleCheckReminders is the system reminder job: it marks every user's
// due reminders notified and reports them.
func (h *handler) handleCheckReminders(w http.ResponseWriter, r *http.Request) {
	work := func(ctx context.Context) (map[string]interface{}, error) {
		pending, err := h.app.Store.GetAllPendingReminders(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range pending {
			if err := h.app.Store.MarkReminderNotified(ctx, p.ReminderID); err != nil {
				return nil, fmt.Errorf("mark reminder %s notified: %w", p.ReminderID, err)
			}
		}
		return map[string]interface{}{
			"checked":   len(pending),
			"reminders": pending,
		}, nil
	}

	if h.app.client.RunAsync(w, r, work) {
		return
	}

	result, err := work(r.Context())
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *handler) handleSetReminders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MinutesBefore []int `json:"minutes_before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid body")
		return
	}
	reminders, err := h.app.Store.SetReminders(r.Context(), client.GetUserID(r.Context()), r.PathValue("id"), req.MinutesBefore)
	if err != nil {
		writeStoreErr(w, r, "event", err)
		return
	}
	writeJSON(w, r, http.StatusOK, reminders)
}

func (h *handler) handleCheckConflicts(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	excludeID := r.URL.Query().Get("exclude_id")
	if startStr == "" || endStr == "" {
		writeErr(w, r, http.StatusBadRequest, "start and end are required")
		return
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid start")
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid end")
		return
	}
	conflicts, err := h.app.Store.GetConflictingEvents(r.Context(), client.GetUserID(r.Context()), start, end, excludeID)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, r, http.StatusOK, conflicts)
}

func (h *handler) handleListPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, CalDAVPresets)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v interface{}) {
	httputil.WriteResponse(w, r, status, v)
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, format string, args ...interface{}) {
	httputil.WriteError(w, r, status, format, args...)
}

// writeStoreErr answers 404 for a record that does not exist or is not the
// caller's, and 500 with the real cause for anything else.
func writeStoreErr(w http.ResponseWriter, r *http.Request, what string, err error) {
	if errors.Is(err, ErrNotFound) {
		writeErr(w, r, http.StatusNotFound, "%s not found", what)
		return
	}
	writeErr(w, r, http.StatusInternalServerError, "%v", err)
}

// writeOAuthPage renders the small popup page the OAuth flow lands on.
func writeOAuthPage(w http.ResponseWriter, title, message string, closeAfterMs int) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><h3>%s</h3><p>%s</p><script>setTimeout(function(){window.close();},%d);</script></body></html>`,
		html.EscapeString(title), html.EscapeString(message), closeAfterMs)
}
