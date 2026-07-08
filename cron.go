package calendar

import (
	"encoding/json"
	"net/http"
)

func HandleCron(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"jobs": []map[string]interface{}{
			{
				"id":          "cron:calendar:caldav-sync",
				"path":        "/api/sync",
				"method":      "POST",
				"schedule":    "*/30 * * * *",
				"description": "Syncs events from all CalDAV calendar accounts",
				"timeout":     "120s",
				"retry": map[string]interface{}{
					"max_attempts":       3,
					"initial_delay":      "5s",
					"max_delay":          "30s",
					"backoff":            "exponential",
					"backoff_multiplier": 2.0,
				},
			},
			{
				"id":          "cron:calendar:reminders",
				"path":        "/api/reminders/check",
				"method":      "POST",
				"schedule":    "* * * * *",
				"description": "Checks for due calendar reminders",
				"timeout":     "10s",
				"retry": map[string]interface{}{
					"max_attempts": 1,
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}
