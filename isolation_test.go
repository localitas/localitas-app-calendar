package calendar

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/localitas/localitas-go"
	_ "github.com/mattn/go-sqlite3"
)

const testAPIToken = "calendar-test-api-token"

// unreachableDAV refuses connections at once, so a sync against it fails
// fast without leaving the machine.
const unreachableDAV = "http://127.0.0.1:1/dav/"

func TestMain(m *testing.M) {
	os.Setenv("LOCALITAS_API_TOKEN", testAPIToken)
	os.Setenv("LOCALITAS_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	client.SetIdentityKeyFromAPIToken(testAPIToken)
	os.Exit(m.Run())
}

// identity signs claims the way core does for a request it forwards.
func identity(apiToken, userID string, scope client.Scope) string {
	raw, _ := json.Marshal(client.TokenInfo{UserID: userID, Email: userID + "@example.com", Permission: string(scope)})
	payload := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte("localitas-internal-token-v1:" + apiToken))
	mac := hmac.New(sha256.New, []byte(base64.StdEncoding.EncodeToString(sum[:])))
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var (
	alice = identity(testAPIToken, "alice", client.ScopeWrite)
	bob   = identity(testAPIToken, "bob", client.ScopeWrite)
)

type testEnv struct {
	t     *testing.T
	store *Store
	mux   *http.ServeMux
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "calendar.sqlitedb")+"?_busy_timeout=10000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	migrations := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationsFS, "_migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := fs.ReadFile(migrationsFS, "_migrations/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		migrations["migrations/"+e.Name()] = &fstest.MapFile{Data: data}
	}
	m, err := client.NewMigrator(db, migrations, DatabaseName)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	store := NewStore(db)
	app := &App{Store: store, CalDAV: NewCalDAVBackend(store), BasePath: "/"}
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	return &testEnv{t: t, store: store, mux: mux}
}

func (e *testEnv) do(method, path, token, body string, headers ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, req)
	return w
}

func (e *testEnv) expect(w *httptest.ResponseRecorder, status int, what string) {
	e.t.Helper()
	if w.Code != status {
		e.t.Fatalf("%s: status %d, want %d: %s", what, w.Code, status, w.Body.String())
	}
}

type seeded struct {
	account    *CalendarAccount
	calendar   *Calendar
	event      *Event
	reminder   *Reminder
	local      *Calendar
	localEvent *Event
}

// seedUser gives userID a CalDAV account with one calendar holding an event
// starting in ten minutes with a due reminder, and a local event.
func (e *testEnv) seedUser(userID string) seeded {
	e.t.Helper()
	ctx := context.Background()
	acct, err := e.store.CreateAccount(ctx, userID, userID+" dav", "caldav", "", unreachableDAV, userID, "pw-"+userID, "", "", "", "")
	if err != nil {
		e.t.Fatal(err)
	}
	cal, err := e.store.UpsertCalendar(ctx, acct.ID, "Work", "/dav/"+userID+"/work/", "#60a5fa")
	if err != nil {
		e.t.Fatal(err)
	}
	start := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Second)
	ev, err := e.store.Create(ctx, cal, "uid-"+userID+"@example.com", userID+" standup", "", "", "", start, start.Add(time.Hour), false)
	if err != nil {
		e.t.Fatal(err)
	}
	rem, err := e.store.AddReminder(ctx, userID, ev.ID, 15)
	if err != nil {
		e.t.Fatal(err)
	}
	local, err := e.store.EnsureLocalCalendar(ctx, userID)
	if err != nil {
		e.t.Fatal(err)
	}
	localEv, err := e.store.Create(ctx, local, newEventUID(), userID+" errand", "", "", "", start.Add(2*time.Hour), start.Add(3*time.Hour), false)
	if err != nil {
		e.t.Fatal(err)
	}
	return seeded{account: acct, calendar: cal, event: ev, reminder: rem, local: local, localEvent: localEv}
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return v
}

func eventIDs(t *testing.T, w *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	resp := decode[struct {
		Days []DayEvents `json:"days"`
	}](t, w)
	ids := map[string]bool{}
	for _, d := range resp.Days {
		for _, ev := range d.Events {
			ids[ev.ID] = true
		}
	}
	return ids
}

func TestUserRoutesRefuseRequestsWithoutVerifiedUser(t *testing.T) {
	env := newTestEnv(t)
	forged := identity("some-other-cluster", "alice", client.ScopeWrite)
	routes := []struct{ method, path string }{
		{"GET", "/api/events"},
		{"POST", "/api/events"},
		{"GET", "/api/events/x"},
		{"PUT", "/api/events/x"},
		{"DELETE", "/api/events/x"},
		{"GET", "/api/search?q=x"},
		{"GET", "/api/parse-date?q=today"},
		{"GET", "/api/accounts"},
		{"POST", "/api/accounts"},
		{"PUT", "/api/accounts/x"},
		{"DELETE", "/api/accounts/x"},
		{"POST", "/api/accounts/x/sync"},
		{"POST", "/api/accounts/sync"},
		{"GET", "/api/oauth/x/start"},
		{"GET", "/api/oauth/callback?code=c&state=x"},
		{"GET", "/api/calendars"},
		{"PUT", "/api/calendars/x"},
		{"POST", "/api/events/x/reminders"},
		{"PUT", "/api/events/x/reminders"},
		{"GET", "/api/events/x/reminders"},
		{"DELETE", "/api/reminders/x"},
		{"GET", "/api/reminders/pending"},
		{"GET", "/api/conflicts?start=x&end=y"},
		{"GET", "/api/presets"},
		{"PROPFIND", "/caldav/me/calendars/"},
		{"GET", "/caldav/me/calendars/x/y.ics"},
	}
	for _, rt := range routes {
		env.expect(env.do(rt.method, rt.path, "", ""), http.StatusUnauthorized, rt.method+" "+rt.path+" without token")
		env.expect(env.do(rt.method, rt.path, forged, ""), http.StatusUnauthorized, rt.method+" "+rt.path+" with forged token")
		env.expect(env.do(rt.method, rt.path, testAPIToken, ""), http.StatusUnauthorized, rt.method+" "+rt.path+" with the system token")
	}
}

func TestReadScopeCannotChangeAnything(t *testing.T) {
	env := newTestEnv(t)
	a := env.seedUser("alice")
	reader := identity(testAPIToken, "alice", client.ScopeRead)

	env.expect(env.do("GET", "/api/events/"+a.event.ID, reader, ""), http.StatusOK, "reader reads own event")
	env.expect(env.do("POST", "/api/events", reader, `{"title":"x"}`), http.StatusForbidden, "reader creates event")
	env.expect(env.do("DELETE", "/api/events/"+a.event.ID, reader, ""), http.StatusForbidden, "reader deletes event")
	env.expect(env.do("PUT", "/api/accounts/"+a.account.ID, reader, `{"name":"x"}`), http.StatusForbidden, "reader edits account")
	env.expect(env.do("PUT", calendarPath(a.calendar.ID)+"new.ics", reader, generateICS("new", "x", "", "", "", time.Now(), time.Now().Add(time.Hour), false), "Content-Type", "text/calendar"),
		http.StatusForbidden, "reader puts CalDAV object")
}

func TestEventsAreVisibleOnlyToTheirOwner(t *testing.T) {
	env := newTestEnv(t)
	a := env.seedUser("alice")
	b := env.seedUser("bob")

	for _, id := range []string{a.event.ID, a.localEvent.ID, "no-such-event"} {
		env.expect(env.do("GET", "/api/events/"+id, bob, ""), http.StatusNotFound, "bob reads "+id)
		env.expect(env.do("PUT", "/api/events/"+id, bob, `{"title":"hijacked","start_time":"2026-01-01T10:00:00Z","end_time":"2026-01-01T11:00:00Z"}`), http.StatusNotFound, "bob updates "+id)
		env.expect(env.do("DELETE", "/api/events/"+id, bob, ""), http.StatusNotFound, "bob deletes "+id)
	}
	got, err := env.store.Get(context.Background(), "alice", a.event.ID)
	if err != nil || got.Title != "alice standup" {
		t.Fatalf("alice's event after bob's attempts: %+v, %v", got, err)
	}

	w := env.do("GET", "/api/events", bob, "")
	env.expect(w, http.StatusOK, "bob lists events")
	ids := eventIDs(t, w)
	if ids[a.event.ID] || ids[a.localEvent.ID] || !ids[b.event.ID] || !ids[b.localEvent.ID] {
		t.Fatalf("bob's event list = %v", ids)
	}

	w = env.do("GET", "/api/search?q=standup", bob, "")
	env.expect(w, http.StatusOK, "bob searches")
	found := decode[[]Event](t, w)
	if len(found) != 1 || found[0].ID != b.event.ID {
		t.Fatalf("bob's search = %+v", found)
	}

	q := url.Values{
		"start": {a.event.StartTime.Format(time.RFC3339)},
		"end":   {a.event.EndTime.Format(time.RFC3339)},
	}
	w = env.do("GET", "/api/conflicts?"+q.Encode(), bob, "")
	env.expect(w, http.StatusOK, "bob checks conflicts")
	conflicts := decode[[]Event](t, w)
	if len(conflicts) != 1 || conflicts[0].ID != b.event.ID {
		t.Fatalf("bob's conflicts = %+v", conflicts)
	}

	env.expect(env.do("GET", "/api/events/"+a.event.ID, alice, ""), http.StatusOK, "alice reads her event")
	w = env.do("PUT", "/api/events/"+a.localEvent.ID, alice, `{"title":"renamed","start_time":"2026-11-01T10:00:00Z","end_time":"2026-11-01T11:00:00Z"}`)
	env.expect(w, http.StatusOK, "alice updates her event")
	if ev := decode[Event](t, w); ev.Title != "renamed" {
		t.Fatalf("alice's updated event = %+v", ev)
	}
	env.expect(env.do("DELETE", "/api/events/"+a.localEvent.ID, alice, ""), http.StatusOK, "alice deletes her event")
	env.expect(env.do("GET", "/api/events/"+a.localEvent.ID, alice, ""), http.StatusNotFound, "alice reads her deleted event")
}

func TestEventCannotBeCreatedInAnotherUsersCalendar(t *testing.T) {
	env := newTestEnv(t)
	a := env.seedUser("alice")

	body := `{"title":"planted","start_time":"2026-11-01T10:00:00Z","end_time":"2026-11-01T11:00:00Z","calendar_id":"` + a.calendar.ID + `"}`
	env.expect(env.do("POST", "/api/events", bob, body), http.StatusNotFound, "bob creates in alice's calendar")
	events, err := env.store.GetEventsByCalendarID(context.Background(), "alice", a.calendar.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("alice's calendar after bob's attempt: %d events, %v", len(events), err)
	}

	w := env.do("POST", "/api/events", alice, `{"title":"dinner","start_time":"2026-11-01T18:00:00Z","end_time":"2026-11-01T19:00:00Z"}`)
	env.expect(w, http.StatusCreated, "alice creates a local event")
	created := decode[Event](t, w)
	if created.CalendarID != a.local.ID {
		t.Fatalf("alice's new event is in calendar %q, want her local calendar %q", created.CalendarID, a.local.ID)
	}
	env.expect(env.do("GET", "/api/events/"+created.ID, bob, ""), http.StatusNotFound, "bob reads alice's new event")
}

func TestAccountsAreVisibleOnlyToTheirOwner(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.seedUser("alice")
	env.seedUser("bob")

	repoint := `{"name":"mine now","provider":"caldav","caldav_url":"https://attacker.example/dav/","username":"alice"}`
	env.expect(env.do("PUT", "/api/accounts/"+a.account.ID, bob, repoint), http.StatusNotFound, "bob repoints alice's account")
	env.expect(env.do("POST", "/api/accounts/"+a.account.ID+"/sync", bob, ""), http.StatusNotFound, "bob syncs alice's account")
	env.expect(env.do("DELETE", "/api/accounts/"+a.account.ID, bob, ""), http.StatusNotFound, "bob deletes alice's account")
	env.expect(env.do("PUT", "/api/accounts/"+a.local.AccountID, alice, repoint), http.StatusNotFound, "alice edits her hidden local account")

	acct, err := env.store.GetAccount(ctx, "alice", a.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if acct.CalDAVURL != unreachableDAV || acct.Password != "pw-alice" {
		t.Fatalf("alice's account after bob's attempts: url %q password %q", acct.CalDAVURL, acct.Password)
	}

	w := env.do("GET", "/api/accounts", bob, "")
	env.expect(w, http.StatusOK, "bob lists accounts")
	for _, listed := range decode[[]CalendarAccount](t, w) {
		if listed.ID == a.account.ID {
			t.Fatal("bob's account list includes alice's account")
		}
	}

	env.expect(env.do("PUT", "/api/accounts/"+a.account.ID, alice, `{"name":"Work DAV","provider":"caldav","caldav_url":"`+unreachableDAV+`","username":"alice"}`), http.StatusOK, "alice edits her account")
	env.expect(env.do("DELETE", "/api/accounts/"+a.account.ID, alice, ""), http.StatusOK, "alice deletes her account")
	if _, err := env.store.Get(ctx, "alice", a.event.ID); err != ErrNotFound {
		t.Fatalf("event of deleted account: %v, want ErrNotFound", err)
	}
}

func TestOAuthFlowOnlyTouchesTheCallersAccount(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	google, err := env.store.CreateAccount(ctx, "alice", "Google", "google", "alice@example.com", "", "", "", "client-id", "client-secret", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SaveOAuthTokens(ctx, "alice", google.ID, "access-a", "refresh-a", 3600); err != nil {
		t.Fatal(err)
	}

	env.expect(env.do("GET", "/api/oauth/"+google.ID+"/start", bob, ""), http.StatusNotFound, "bob starts OAuth for alice's account")
	env.expect(env.do("GET", "/api/oauth/callback?code=attacker-code&state="+google.ID, bob, ""), http.StatusNotFound, "bob completes OAuth for alice's account")

	full, err := env.store.GetAccountWithTokens(ctx, "alice", google.ID)
	if err != nil {
		t.Fatal(err)
	}
	if full.AccessToken != "access-a" || full.RefreshToken != "refresh-a" {
		t.Fatalf("alice's tokens after bob's callback: %q %q", full.AccessToken, full.RefreshToken)
	}

	w := env.do("GET", "/api/oauth/"+google.ID+"/start", alice, "")
	env.expect(w, http.StatusOK, "alice starts OAuth")
	if start := decode[map[string]string](t, w); !strings.Contains(start["auth_url"], "state="+google.ID) {
		t.Fatalf("auth_url = %q", start["auth_url"])
	}
}

func TestCalendarsAreVisibleOnlyToTheirOwner(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.seedUser("alice")
	b := env.seedUser("bob")

	w := env.do("GET", "/api/calendars", bob, "")
	env.expect(w, http.StatusOK, "bob lists calendars")
	for _, c := range decode[[]Calendar](t, w) {
		if c.ID == a.calendar.ID || c.ID == a.local.ID {
			t.Fatalf("bob's calendar list includes alice's %s", c.ID)
		}
	}
	w = env.do("GET", "/api/calendars?account_id="+a.account.ID, bob, "")
	env.expect(w, http.StatusOK, "bob lists alice's account calendars")
	if cals := decode[[]Calendar](t, w); len(cals) != 0 {
		t.Fatalf("bob sees alice's account calendars: %+v", cals)
	}

	env.expect(env.do("PUT", "/api/calendars/"+a.calendar.ID, bob, `{"color":"#000000","is_visible":false}`), http.StatusNotFound, "bob edits alice's calendar")
	cal, err := env.store.GetCalendar(ctx, "alice", a.calendar.ID)
	if err != nil || cal.Color != "#60a5fa" || !cal.IsVisible {
		t.Fatalf("alice's calendar after bob's attempt: %+v, %v", cal, err)
	}

	env.expect(env.do("PUT", "/api/calendars/"+b.calendar.ID, bob, `{"color":"#123456"}`), http.StatusOK, "bob edits his calendar")
	if cal, _ := env.store.GetCalendar(ctx, "bob", b.calendar.ID); cal.Color != "#123456" {
		t.Fatalf("bob's calendar color = %q", cal.Color)
	}
}

func TestRemindersAreVisibleOnlyToTheirOwner(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.seedUser("alice")
	b := env.seedUser("bob")

	env.expect(env.do("POST", "/api/events/"+a.event.ID+"/reminders", bob, `{"minutes_before":5}`), http.StatusNotFound, "bob adds reminder")
	env.expect(env.do("PUT", "/api/events/"+a.event.ID+"/reminders", bob, `{"minutes_before":[]}`), http.StatusNotFound, "bob replaces reminders")
	env.expect(env.do("GET", "/api/events/"+a.event.ID+"/reminders", bob, ""), http.StatusNotFound, "bob lists reminders")
	env.expect(env.do("DELETE", "/api/reminders/"+a.reminder.ID, bob, ""), http.StatusNotFound, "bob deletes reminder")

	rems, err := env.store.ListReminders(ctx, "alice", a.event.ID)
	if err != nil || len(rems) != 1 {
		t.Fatalf("alice's reminders after bob's attempts: %+v, %v", rems, err)
	}

	w := env.do("GET", "/api/reminders/pending", bob, "")
	env.expect(w, http.StatusOK, "bob's pending reminders")
	pending := decode[[]PendingReminder](t, w)
	if len(pending) != 1 || pending[0].ReminderID != b.reminder.ID {
		t.Fatalf("bob's pending reminders = %+v", pending)
	}

	w = env.do("PUT", "/api/events/"+a.event.ID+"/reminders", alice, `{"minutes_before":[10,30]}`)
	env.expect(w, http.StatusOK, "alice replaces her reminders")
	if set := decode[[]Reminder](t, w); len(set) != 2 {
		t.Fatalf("alice's reminders = %+v", set)
	}
}

func TestCronRoutesAreSystemOnlyAndActForEveryUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.seedUser("alice")
	b := env.seedUser("bob")

	for _, path := range []string{"/api/reminders/check", "/api/sync"} {
		env.expect(env.do("POST", path, alice, ""), http.StatusForbidden, "alice calls "+path)
		env.expect(env.do("POST", path, "", ""), http.StatusForbidden, "anonymous call to "+path)
	}
	if rems, _ := env.store.GetAllPendingReminders(ctx); len(rems) != 2 {
		t.Fatalf("pending reminders after refused calls = %d, want 2", len(rems))
	}

	w := env.do("POST", "/api/reminders/check", testAPIToken, "")
	env.expect(w, http.StatusOK, "system checks reminders")
	if checked := decode[struct {
		Checked int `json:"checked"`
	}](t, w); checked.Checked != 2 {
		t.Fatalf("checked %d reminders, want both users' 2", checked.Checked)
	}
	if rems, _ := env.store.GetAllPendingReminders(ctx); len(rems) != 0 {
		t.Fatalf("pending reminders after check = %d", len(rems))
	}

	w = env.do("POST", "/api/sync", testAPIToken, "")
	env.expect(w, http.StatusOK, "system syncs")
	if synced := decode[struct {
		Accounts int      `json:"accounts"`
		Errors   []string `json:"errors"`
	}](t, w); synced.Accounts != 2 || len(synced.Errors) != 2 {
		t.Fatalf("system sync = %+v, want both users' accounts attempted", synced)
	}
	for user, acct := range map[string]*CalendarAccount{"alice": a.account, "bob": b.account} {
		got, err := env.store.GetAccount(ctx, user, acct.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.LastSyncedAt == nil || got.SyncError == "" {
			t.Fatalf("%s's account was not synced: %+v", user, got)
		}
	}
}

func TestUserSyncTouchesOnlyTheCallersAccounts(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.seedUser("alice")
	b := env.seedUser("bob")

	w := env.do("POST", "/api/accounts/sync", alice, "")
	env.expect(w, http.StatusOK, "alice syncs her accounts")
	if synced := decode[struct {
		Accounts int `json:"accounts"`
	}](t, w); synced.Accounts != 1 {
		t.Fatalf("alice's sync covered %d accounts, want 1", synced.Accounts)
	}
	got, err := env.store.GetAccount(ctx, "bob", b.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSyncedAt != nil {
		t.Fatal("alice's sync touched bob's account")
	}
}

const propfindDisplayName = `<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:displayname/></d:prop></d:propfind>`

func TestCalDAVServesOnlyTheCallersCalendars(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.seedUser("alice")
	b := env.seedUser("bob")

	w := env.do("PROPFIND", caldavHome, alice, propfindDisplayName, "Depth", "1", "Content-Type", "application/xml")
	env.expect(w, http.StatusMultiStatus, "alice lists her calendars")
	listing := w.Body.String()
	for _, mine := range []string{calendarPath(a.calendar.ID), calendarPath(a.local.ID)} {
		if !strings.Contains(listing, mine) {
			t.Fatalf("alice's listing misses %s: %s", mine, listing)
		}
	}
	for _, theirs := range []string{b.calendar.ID, b.local.ID} {
		if strings.Contains(listing, theirs) {
			t.Fatalf("alice's listing shows bob's calendar %s", theirs)
		}
	}

	object := calendarPath(a.calendar.ID) + a.event.RemoteUID + ".ics"
	w = env.do("GET", object, alice, "")
	env.expect(w, http.StatusOK, "alice gets her object")
	if !strings.Contains(w.Body.String(), "alice standup") {
		t.Fatalf("alice's object = %s", w.Body.String())
	}

	env.expect(env.do("GET", object, bob, ""), http.StatusNotFound, "bob gets alice's object")
	env.expect(env.do("PROPFIND", calendarPath(a.calendar.ID), bob, propfindDisplayName, "Depth", "1", "Content-Type", "application/xml"), http.StatusNotFound, "bob lists alice's calendar")
	start := time.Now().UTC().Add(24 * time.Hour)
	planted := generateICS("planted@x", "planted", "", "", "", start, start.Add(time.Hour), false)
	env.expect(env.do("PUT", calendarPath(a.calendar.ID)+"planted@x.ics", bob, planted, "Content-Type", "text/calendar"), http.StatusNotFound, "bob puts into alice's calendar")
	env.expect(env.do("DELETE", object, bob, ""), http.StatusNotFound, "bob deletes alice's object")
	if events, err := env.store.GetEventsByCalendarID(ctx, "alice", a.calendar.ID); err != nil || len(events) != 1 {
		t.Fatalf("alice's calendar after bob's attempts: %d events, %v", len(events), err)
	}

	mine := generateICS("lunch@x", "lunch", "", "", "", start, start.Add(time.Hour), false)
	env.expect(env.do("PUT", calendarPath(a.calendar.ID)+"lunch@x.ics", alice, mine, "Content-Type", "text/calendar"), http.StatusCreated, "alice puts into her calendar")
	if _, err := env.store.GetEventByRemoteUID(ctx, "alice", a.calendar.ID, "lunch@x"); err != nil {
		t.Fatalf("alice's put object: %v", err)
	}
}
