package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleCron(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var spec struct {
		Jobs []struct {
			ID       string `json:"id"`
			Path     string `json:"path"`
			Method   string `json:"method"`
			Schedule string `json:"schedule"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(spec.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(spec.Jobs))
	}

	if spec.Jobs[0].ID != "cron:calendar:caldav-sync" {
		t.Errorf("expected first job id cron:calendar:caldav-sync, got %s", spec.Jobs[0].ID)
	}
	if spec.Jobs[0].Schedule != "*/30 * * * *" {
		t.Errorf("expected schedule */30 * * * *, got %s", spec.Jobs[0].Schedule)
	}

	if spec.Jobs[1].ID != "cron:calendar:reminders" {
		t.Errorf("expected second job id cron:calendar:reminders, got %s", spec.Jobs[1].ID)
	}
	if spec.Jobs[1].Schedule != "* * * * *" {
		t.Errorf("expected schedule * * * * *, got %s", spec.Jobs[1].Schedule)
	}
}

func TestHandleCron_ContentType(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestHandleCron_TwoJobs(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	var spec struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(spec.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(spec.Jobs))
	}
}

func TestHandleCron_AllJobsHaveID(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	var spec struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for i, job := range spec.Jobs {
		if job.ID == "" {
			t.Errorf("job[%d]: ID is empty", i)
		}
		if !strings.HasPrefix(job.ID, "cron:") {
			t.Errorf("job[%d]: expected ID to start with cron:, got %s", i, job.ID)
		}
	}
}

func TestHandleCron_AllMethodsPost(t *testing.T) {
	req := httptest.NewRequest("GET", "/cron.json", nil)
	w := httptest.NewRecorder()
	HandleCron(w, req)

	var spec struct {
		Jobs []struct {
			Method string `json:"method"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for i, job := range spec.Jobs {
		if job.Method != "POST" {
			t.Errorf("job[%d]: expected method POST, got %s", i, job.Method)
		}
	}
}
