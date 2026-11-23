package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func parseDate(t *testing.T, q, accept string) string {
	t.Helper()
	h := &handler{app: &App{}}
	req := httptest.NewRequest(http.MethodGet, "/api/parse-date?q="+q, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.handleParseDate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestParseDateResponse_Bytes(t *testing.T) {
	cases := []struct {
		name, q, accept, want string
	}{
		{"parsed json", "Jan+15,+2026", "", `{"date":"2026-01-15","input":"Jan 15, 2026","label":"Thursday, January 15, 2026","parsed":true}` + "\n"},
		{"unparsed json", "nonsense", "", `{"input":"nonsense","parsed":false}` + "\n"},
		{"parsed yaml", "Jan+15,+2026", "application/yaml", "date: \"2026-01-15\"\ninput: Jan 15, 2026\nlabel: Thursday, January 15, 2026\nparsed: true\n"},
		{"unparsed yaml", "nonsense", "application/yaml", "input: nonsense\nparsed: false\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseDate(t, c.q, c.accept); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestEventsAndSyncResponses_Bytes(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"events", eventsResponse{Days: []DayEvents{}}, `{"days":[]}`},
		{"no events", eventsResponse{}, `{"days":null}`},
		{"sync", syncAccountResponse{NewEvents: 3}, `{"new_events":3}`},
	}
	for _, c := range cases {
		b, err := json.Marshal(c.v)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != c.want {
			t.Errorf("%s: got %s, want %s", c.name, b, c.want)
		}
	}
}
