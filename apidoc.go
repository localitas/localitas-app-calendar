package calendar

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type APIEndpoint struct {
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	Summary     string     `json:"summary"`
	QueryParams []APIParam `json:"query_params,omitempty"`
	RequestBody *APIBody   `json:"request_body,omitempty"`
	Response    *APIBody   `json:"response,omitempty"`
}

type APIParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type APIBody struct {
	ContentType string `json:"content_type"`
	Example     string `json:"example"`
}

type APIFieldDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

type APIDoc struct {
	AppName     string        `json:"app_name"`
	Version     string        `json:"version"`
	Description string        `json:"description"`
	Keywords    []string      `json:"keywords,omitempty"`
	Fields      []APIFieldDoc `json:"fields,omitempty"`
	Endpoints   []APIEndpoint `json:"endpoints"`
}

var CalendarAPIDoc = APIDoc{
	AppName:     "Calendar",
	Version:     "0.1.0",
	Description: "Event calendar with CRUD, date range queries, natural date parsing, and full-text search",
	Keywords:    []string{"calendar", "event", "schedule", "appointment", "meeting", "reminder", "date", "time", "agenda", "booking", "availability"},
	Fields: []APIFieldDoc{
		{Name: "Date Formats", Description: "Supported date/time formats for creating events", Example: "RFC3339: 2026-04-23T14:00:00Z\nISO date: 2026-04-23\nDatetime-local: 2026-04-23T14:00"},
		{Name: "Natural Dates", Description: "The parse-date endpoint understands natural language", Example: "today, tomorrow, yesterday\nnext monday, last friday\n3 days from now, 2 weeks ago\nJan 15, 2026"},
		{Name: "All-Day Events", Description: "Set all_day: true for events spanning entire days", Example: "all_day: true\nstart_time: 2026-04-23\nend_time: 2026-04-23"},
	},
	Endpoints: []APIEndpoint{
		{
			Method:  "GET",
			Path:    "/api/events",
			Summary: "List events by date range",
			QueryParams: []APIParam{
				{Name: "start", Type: "string", Description: "Start date YYYY-MM-DD (default: 7 days ago)"},
				{Name: "end", Type: "string", Description: "End date YYYY-MM-DD (default: 14 days from now)"},
			},
			Response: &APIBody{ContentType: "application/json", Example: `{"days":[{"date":"2026-04-23","date_label":"Wednesday, April 23","events":[{"id":"abc...","title":"Meeting","start_time":"2026-04-23T14:00:00Z"}]}]}`},
		},
		{
			Method:      "POST",
			Path:        "/api/events",
			Summary:     "Create an event",
			RequestBody: &APIBody{ContentType: "application/json", Example: `{"title":"Team Meeting","description":"Weekly sync","start_time":"2026-04-23T14:00:00Z","end_time":"2026-04-23T15:00:00Z","location":"Room 3","all_day":false}`},
			Response:    &APIBody{ContentType: "application/json", Example: `{"id":"abc...","title":"Team Meeting","start_time":"2026-04-23T14:00:00Z","end_time":"2026-04-23T15:00:00Z"}`},
		},
		{
			Method:   "GET",
			Path:     "/api/events/{id}",
			Summary:  "Get an event by ID",
			Response: &APIBody{ContentType: "application/json", Example: `{"id":"abc...","title":"Team Meeting","description":"Weekly sync","start_time":"2026-04-23T14:00:00Z"}`},
		},
		{
			Method:      "PUT",
			Path:        "/api/events/{id}",
			Summary:     "Update an event",
			RequestBody: &APIBody{ContentType: "application/json", Example: `{"title":"Updated Meeting","start_time":"2026-04-23T15:00:00Z","end_time":"2026-04-23T16:00:00Z"}`},
			Response:    &APIBody{ContentType: "application/json", Example: `{"id":"abc...","title":"Updated Meeting"}`},
		},
		{
			Method:   "DELETE",
			Path:     "/api/events/{id}",
			Summary:  "Delete an event",
			Response: &APIBody{ContentType: "application/json", Example: `{"success":true}`},
		},
		{
			Method:  "GET",
			Path:    "/api/search",
			Summary: "Search events (FTS5)",
			QueryParams: []APIParam{
				{Name: "q", Type: "string", Required: true, Description: "Search query"},
			},
			Response: &APIBody{ContentType: "application/json", Example: `[{"id":"abc...","title":"Team Meeting","location":"Room 3"}]`},
		},
		{
			Method:  "GET",
			Path:    "/api/parse-date",
			Summary: "Parse natural language date",
			QueryParams: []APIParam{
				{Name: "q", Type: "string", Required: true, Description: "Date expression (e.g. 'next friday', 'Jan 15')"},
			},
			Response: &APIBody{ContentType: "application/json", Example: `{"parsed":true,"input":"next friday","date":"2026-04-25","label":"Friday, April 25, 2026"}`},
		},
		{
			Method:  "PROPFIND",
			Path:    "/caldav/",
			Summary: "CalDAV discovery — list calendars",
		},
		{
			Method:  "REPORT",
			Path:    "/caldav/{calendar_id}/",
			Summary: "CalDAV query — list/filter events by time range",
		},
		{
			Method:  "PUT",
			Path:    "/caldav/{calendar_id}/{uid}.ics",
			Summary: "CalDAV create/update event via iCalendar",
		},
		{
			Method:  "DELETE",
			Path:    "/caldav/{calendar_id}/{uid}.ics",
			Summary: "CalDAV delete event",
		},
	},
}

func HandleSwagger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CalendarAPIDoc)
}

func RenderDocsHTML(doc APIDoc) template.HTML {
	var sb strings.Builder
	if len(doc.Fields) > 0 {
		sb.WriteString(`<h3 style="font-size: 0.875rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-secondary); margin-bottom: 1rem;">Reference</h3><div class="accordion-list">`)
		for _, f := range doc.Fields {
			sb.WriteString(fmt.Sprintf(`<details class="glass-panel" style="border-radius: 0.5rem; margin-bottom: 0.5rem;"><summary style="padding: 0.75rem 1rem; cursor: pointer; font-weight: 500; color: var(--color-text-primary);">%s</summary><div style="padding: 0 1rem 0.75rem; font-size: 0.875rem; color: var(--color-text-secondary);"><p style="margin-bottom: 0.5rem;">%s</p><pre style="background: var(--color-bg-base); padding: 0.75rem; border-radius: 0.375rem; overflow-x: auto; font-size: 0.8125rem;">%s</pre></div></details>`, template.HTMLEscapeString(f.Name), template.HTMLEscapeString(f.Description), template.HTMLEscapeString(f.Example)))
		}
		sb.WriteString(`</div><hr style="border-color: var(--color-glass-border); margin: 1.5rem 0;">`)
	}
	sb.WriteString(`<h3 style="font-size: 0.875rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--color-text-secondary); margin-bottom: 1rem;">API Endpoints</h3><div class="accordion-list">`)
	for _, ep := range doc.Endpoints {
		title := fmt.Sprintf("%s %s — %s", ep.Method, ep.Path, ep.Summary)
		sb.WriteString(fmt.Sprintf(`<details class="glass-panel" style="border-radius: 0.5rem; margin-bottom: 0.5rem;"><summary style="padding: 0.75rem 1rem; cursor: pointer; font-weight: 500; color: var(--color-text-primary);">%s</summary><div style="padding: 0 1rem 0.75rem; font-size: 0.875rem; color: var(--color-text-secondary);">`, template.HTMLEscapeString(title)))
		var ex strings.Builder
		if ep.RequestBody != nil {
			ex.WriteString("# Request\n")
			ex.WriteString(prettyJSON(ep.RequestBody.Example))
			ex.WriteString("\n\n")
		}
		if ep.Response != nil {
			ex.WriteString("# Response\n")
			ex.WriteString(prettyJSON(ep.Response.Example))
		}
		if ex.Len() > 0 {
			sb.WriteString(fmt.Sprintf(`<pre style="background: var(--color-bg-base); padding: 0.75rem; border-radius: 0.375rem; overflow-x: auto; font-size: 0.8125rem;">%s</pre>`, template.HTMLEscapeString(ex.String())))
		}
		sb.WriteString(`</div></details>`)
	}
	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

func prettyJSON(s string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	return string(b)
}
