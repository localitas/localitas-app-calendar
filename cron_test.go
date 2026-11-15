package calendar

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	client "github.com/localitas/localitas-go"
)

func strictDecode(t *testing.T, data []byte, into any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
}

// assertWireKeys pins a response's top-level JSON keys, in order: a client
// reads them by name, so a renamed field is a broken response even when the
// body still decodes into its own type.
func assertWireKeys(t *testing.T, data []byte, want ...string) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("%s is not a JSON object", data)
	}
	var got []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("decode %s: %v", data, err)
		}
		got = append(got, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("decode %s: %v", data, err)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("response keys %v, want %v", got, want)
	}
}

func assertCronJob(t *testing.T, got, want client.CronJob) {
	t.Helper()
	if got.ID != want.ID || got.Path != want.Path || got.Method != want.Method || got.Schedule != want.Schedule ||
		got.Description != want.Description || got.Timeout != want.Timeout || got.NotifyLevel != want.NotifyLevel ||
		got.ExecutionStrategy != want.ExecutionStrategy || len(got.Headers) != len(want.Headers) || len(got.Body) != len(want.Body) {
		t.Fatalf("job %+v, want %+v", got, want)
	}
	if (got.Retry == nil) != (want.Retry == nil) || (got.Retry != nil && *got.Retry != *want.Retry) {
		t.Fatalf("job %s retry %+v, want %+v", want.ID, got.Retry, want.Retry)
	}
	if (got.Auth == nil) != (want.Auth == nil) || (got.Auth != nil && *got.Auth != *want.Auth) {
		t.Fatalf("job %s auth %+v, want %+v", want.ID, got.Auth, want.Auth)
	}
}

func TestHandleCron_TypedJobs(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleCron(rec, httptest.NewRequest("GET", "/cron.json", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("cron.json: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	var spec client.CronSpec
	strictDecode(t, rec.Body.Bytes(), &spec)
	if len(spec.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(spec.Jobs))
	}
	assertCronJob(t, spec.Jobs[0], client.CronJob{
		ID:          "cron:calendar:caldav-sync",
		Path:        "/api/sync",
		Method:      "POST",
		Schedule:    "*/30 * * * *",
		Description: "Syncs events from all CalDAV calendar accounts",
		Timeout:     "120s",
		Retry: &client.CronRetry{
			MaxAttempts:       3,
			InitialDelay:      "5s",
			MaxDelay:          "30s",
			Backoff:           "exponential",
			BackoffMultiplier: 2.0,
		},
	})
	assertCronJob(t, spec.Jobs[1], client.CronJob{
		ID:          "cron:calendar:reminders",
		Path:        "/api/reminders/check",
		Method:      "POST",
		Schedule:    "* * * * *",
		Description: "Checks for due calendar reminders",
		Timeout:     "10s",
		Retry:       &client.CronRetry{MaxAttempts: 1},
	})
}
