package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/localitas/localitas-go"
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
			writeErr(w, http.StatusBadRequest, "invalid start date")
			return
		}
	} else {
		start = time.Now().AddDate(0, 0, -7)
	}

	if endStr != "" {
		end, err = time.Parse("2006-01-02", endStr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid end date")
			return
		}
		end = end.Add(24*time.Hour - time.Second)
	} else {
		end = time.Now().AddDate(0, 0, 14)
	}

	days, err := h.app.CalDAV.GetEventsByDay(r.Context(), start, end)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to get events: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"days": days})
}

func (h *handler) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := h.app.CalDAV.GetEvent(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "event not found: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, e)
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
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
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

	e, err := h.app.CalDAV.CreateEventJSON(r.Context(), req.CalendarID, req.Title, req.Description, req.Location, req.Timezone, startTime, endTime, req.AllDay)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create event: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
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
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	startTime, _ := time.Parse(time.RFC3339, req.StartTime)
	endTime, _ := time.Parse(time.RFC3339, req.EndTime)

	e, err := h.app.CalDAV.UpdateEventJSON(r.Context(), id, req.Title, req.Description, req.Location, startTime, endTime, req.AllDay)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handler) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.app.CalDAV.DeleteEventJSON(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete event: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeErr(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	events, err := h.app.CalDAV.SearchEvents(r.Context(), query, 20)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "search failed: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *handler) handleParseDate(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeErr(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	parsed, ok := ParseNaturalDate(query)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{"parsed": false, "input": query})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"parsed": true,
		"input":  query,
		"date":   parsed.Format("2006-01-02"),
		"label":  parsed.Format("Monday, January 2, 2006"),
	})
}

func (h *handler) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	userID := client.UserIDFromRequest(r)
	accounts, err := h.app.Store.ListAccounts(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
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
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Provider == "" {
		req.Provider = "caldav"
	}

	if req.VaultCredentialID != "" && h.app.client != nil {
		secrets, err := h.app.client.WithToken(client.TokenFromRequest(r)).VaultGetSecrets(r.Context(), req.VaultCredentialID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "failed to resolve vault credential: %v", err)
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
	}

	if req.Provider == "google" {
		if req.Email == "" || req.OAuthClientID == "" || req.OAuthClientSecret == "" {
			writeErr(w, http.StatusBadRequest, "email, oauth_client_id, oauth_client_secret required for Google")
			return
		}
	} else {
		if req.CalDAVURL == "" || req.Username == "" || req.Password == "" {
			writeErr(w, http.StatusBadRequest, "caldav_url, username, password are required")
			return
		}
	}
	if req.Name == "" {
		req.Name = req.Email
	}
	userID := client.UserIDFromRequest(r)
	account, err := h.app.Store.CreateAccount(r.Context(), userID, req.Name, req.Provider, req.Email, req.CalDAVURL, req.Username, req.Password, req.OAuthClientID, req.OAuthClientSecret, req.Color, req.VaultCredentialID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
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
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.app.Store.UpdateAccount(r.Context(), id, req.Name, req.Provider, req.Email, req.CalDAVURL, req.Username, req.Password, req.Color, req.VaultCredentialID); err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.app.Store.DeleteAccount(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleSyncAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account, err := h.app.Store.GetAccount(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}

	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(0, 3, 0)

	newCount, err := SyncAccount(r.Context(), h.app.Store, account, from, to)
	if err != nil {
		h.app.Store.UpdateSyncStatus(r.Context(), id, err.Error())
		writeErr(w, http.StatusInternalServerError, "sync failed: %v", err)
		return
	}

	h.app.Store.UpdateSyncStatus(r.Context(), id, "")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"new_events": newCount,
	})
}

func (h *handler) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	userID := client.UserIDFromRequest(r)
	accounts, err := h.app.Store.ListAccounts(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}

	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(0, 3, 0)
	totalNew := 0
	var syncErrors []string

	for _, a := range accounts {
		if !a.IsActive {
			continue
		}
		n, err := SyncAccount(r.Context(), h.app.Store, a, from, to)
		if err != nil {
			h.app.Store.UpdateSyncStatus(r.Context(), a.ID, err.Error())
			syncErrors = append(syncErrors, a.Name+": "+err.Error())
			continue
		}
		h.app.Store.UpdateSyncStatus(r.Context(), a.ID, "")
		totalNew += n
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"new_events": totalNew,
		"errors":     syncErrors,
	})
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
	account, err := h.app.Store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	if account.OAuthClientID == "" {
		writeErr(w, http.StatusBadRequest, "no OAuth client ID configured")
		return
	}
	redirectURI := h.oauthRedirectURI(r)
	authURL := GoogleAuthRedirectURL(account.OAuthClientID, redirectURI, accountID)
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL, "redirect_uri": redirectURI})
}

func (h *handler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	accountID := r.URL.Query().Get("state")
	if code == "" || accountID == "" {
		writeErr(w, http.StatusBadRequest, "missing code or state")
		return
	}

	account, err := h.app.Store.GetAccount(r.Context(), accountID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}

	redirectURI := h.oauthRedirectURI(r)
	tok, err := ExchangeGoogleCode(r.Context(), code, account.OAuthClientID, account.OAuthClientSecret, redirectURI)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><h3>Authorization Failed</h3><p>%s</p><script>setTimeout(function(){window.close();},3000);</script></body></html>`, err.Error())
		return
	}

	refreshToken := tok.RefreshToken
	if refreshToken == "" {
		refreshToken = account.RefreshToken
	}
	h.app.Store.SaveOAuthTokens(r.Context(), accountID, tok.AccessToken, refreshToken, tok.ExpiresIn)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><h3>Google Calendar Connected!</h3><p>You can close this window.</p><script>setTimeout(function(){window.close();},2000);</script></body></html>`)
}

func (h *handler) handleListCalendars(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	var calendars []*Calendar
	var err error
	if accountID != "" {
		calendars, err = h.app.Store.ListCalendars(r.Context(), accountID)
	} else {
		calendars, err = h.app.Store.ListAllCalendars(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, calendars)
}

func (h *handler) handleUpdateCalendar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Color     string `json:"color"`
		IsVisible *bool  `json:"is_visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Color != "" {
		h.app.Store.UpdateCalendarColor(r.Context(), id, req.Color)
	}
	if req.IsVisible != nil {
		h.app.Store.UpdateCalendarVisibility(r.Context(), id, *req.IsVisible)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handleAddReminder(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("id")
	var req struct {
		MinutesBefore int `json:"minutes_before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.MinutesBefore <= 0 {
		req.MinutesBefore = 15
	}
	reminder, err := h.app.Store.AddReminder(r.Context(), eventID, req.MinutesBefore)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to add reminder: %v", err)
		return
	}
	writeJSON(w, http.StatusCreated, reminder)
}

func (h *handler) handleListReminders(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("id")
	reminders, err := h.app.Store.ListReminders(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, reminders)
}

func (h *handler) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("rid")
	if err := h.app.Store.DeleteReminder(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) handlePendingReminders(w http.ResponseWriter, r *http.Request) {
	pending, err := h.app.Store.GetPendingReminders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, pending)
}

func (h *handler) handleCheckReminders(w http.ResponseWriter, r *http.Request) {
	pending, err := h.app.Store.GetPendingReminders(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	for _, p := range pending {
		h.app.Store.MarkReminderNotified(r.Context(), p.ReminderID)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"checked":   len(pending),
		"reminders": pending,
	})
}

func (h *handler) handleSetReminders(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("id")
	var req struct {
		MinutesBefore []int `json:"minutes_before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	existing, _ := h.app.Store.ListReminders(r.Context(), eventID)
	for _, rem := range existing {
		h.app.Store.DeleteReminder(r.Context(), rem.ID)
	}
	reminders := make([]*Reminder, 0)
	for _, mins := range req.MinutesBefore {
		if mins <= 0 {
			continue
		}
		rem, err := h.app.Store.AddReminder(r.Context(), eventID, mins)
		if err != nil {
			continue
		}
		reminders = append(reminders, rem)
	}
	writeJSON(w, http.StatusOK, reminders)
}

func (h *handler) handleListEventReminders(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("event_id")
	if eventID == "" {
		writeErr(w, http.StatusBadRequest, "event_id required")
		return
	}
	reminders, err := h.app.Store.ListReminders(r.Context(), eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	mins := make([]int, 0, len(reminders))
	for _, r := range reminders {
		mins = append(mins, r.MinutesBefore)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"minutes_before": mins})
}

func (h *handler) handleBulkReminders(w http.ResponseWriter, r *http.Request) {
	eventIDs := r.URL.Query()["event_id"]
	result := make(map[string][]int)
	for _, eid := range eventIDs {
		reminders, err := h.app.Store.ListReminders(r.Context(), eid)
		if err != nil {
			continue
		}
		mins := make([]int, 0)
		for _, rem := range reminders {
			mins = append(mins, rem.MinutesBefore)
		}
		result[eid] = mins
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) handleCheckConflicts(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	excludeID := r.URL.Query().Get("exclude_id")
	if startStr == "" || endStr == "" {
		writeErr(w, http.StatusBadRequest, "start and end are required")
		return
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid start")
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid end")
		return
	}
	conflicts, err := h.app.Store.GetConflictingEvents(r.Context(), start, end, excludeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, conflicts)
}

func (h *handler) handleListPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, CalDAVPresets)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, format string, args ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)})
}
