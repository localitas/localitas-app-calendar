package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSwagger_ReturnsValidJSON(t *testing.T) {
	req := httptest.NewRequest("GET", "/swagger.json", nil)
	w := httptest.NewRecorder()
	HandleSwagger(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var spec APIDoc
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if spec.AppName != "Calendar" {
		t.Errorf("expected app_name Calendar, got %q", spec.AppName)
	}
	if len(spec.Endpoints) == 0 {
		t.Error("expected at least one endpoint")
	}

	hasEvents := false
	hasParseDate := false
	for _, ep := range spec.Endpoints {
		if strings.Contains(ep.Path, "/api/events") {
			hasEvents = true
		}
		if strings.Contains(ep.Path, "/api/parse-date") {
			hasParseDate = true
		}
	}
	if !hasEvents {
		t.Error("expected /api/events endpoint")
	}
	if !hasParseDate {
		t.Error("expected /api/parse-date endpoint")
	}
}

func TestRenderDocsHTML_ContainsContent(t *testing.T) {
	html := string(RenderDocsHTML(CalendarAPIDoc))
	if !strings.Contains(html, "API Endpoints") {
		t.Error("expected API Endpoints heading")
	}
	if !strings.Contains(html, "GET /api/events") {
		t.Error("expected GET /api/events")
	}
	if !strings.Contains(html, "parse-date") {
		t.Error("expected parse-date endpoint")
	}
}
