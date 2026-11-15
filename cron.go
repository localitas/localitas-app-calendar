package calendar

import (
	"net/http"

	client "github.com/localitas/localitas-go"
)

var cronSpec = client.CronSpec{
	Jobs: []client.CronJob{
		{
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
		},
		{
			ID:          "cron:calendar:reminders",
			Path:        "/api/reminders/check",
			Method:      "POST",
			Schedule:    "* * * * *",
			Description: "Checks for due calendar reminders",
			Timeout:     "10s",
			Retry:       &client.CronRetry{MaxAttempts: 1},
		},
	},
}

func HandleCron(w http.ResponseWriter, r *http.Request) {
	cronSpec.ServeHTTP(w, r)
}
