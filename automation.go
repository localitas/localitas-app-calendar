package calendar

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const (
	syncAutomationName     = "Calendar: CalDAV Sync"
	reminderAutomationName = "Calendar: Reminders"
)

func RegisterSyncAutomation(coreURL, token, appURL string) {
	registerSyncAutomation(coreURL, token, appURL)
	registerReminderAutomation(coreURL, token, appURL)
}

func registerSyncAutomation(coreURL, token, appURL string) {
	if automationExists(coreURL, token, syncAutomationName) {
		log.Printf("✅ Calendar sync automation already registered (user config preserved)")
		return
	}

	body := map[string]interface{}{
		"name":        syncAutomationName,
		"description": "Syncs events from all CalDAV calendar accounts every 30 minutes",
		"dag_config": map[string]interface{}{
			"dag_id":      "calendar_caldav_sync",
			"name":        "Calendar: CalDAV Sync",
			"description": "Calls the calendar app sync endpoint",
			"nodes": []map[string]interface{}{
				{
					"node_id":            "sync_all",
					"node_type":          "http-api",
					"execution_strategy": "raft-leader",
					"metadata": map[string]interface{}{
						"url":                appURL + "/api/sync",
						"method":             "POST",
						"timeout_ms":         120000,
						"max_retries":        3,
						"backoff_ms":         5000,
						"backoff_multiplier": 2.0,
						"expected_status":    200,
					},
				},
			},
		},
		"trigger_type": "periodic",
		"trigger_config": map[string]interface{}{
			"periodic": map[string]interface{}{
				"schedule":    "*/30 * * * *",
				"timezone":    "Local",
				"max_retries": 2,
			},
		},
		"is_enabled": true,
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", coreURL+"/apps/automation/api/automations", bytes.NewReader(b))
	if err != nil {
		log.Printf("⚠️  Failed to create calendar automation request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️  Failed to register calendar sync automation: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("✅ Registered calendar sync automation (every 30 min)")
	} else {
		log.Printf("⚠️  Calendar automation registration returned %d", resp.StatusCode)
	}
}

func registerReminderAutomation(coreURL, token, appURL string) {
	if automationExists(coreURL, token, reminderAutomationName) {
		log.Printf("✅ Calendar reminder automation already registered")
		return
	}

	body := map[string]interface{}{
		"name":        reminderAutomationName,
		"description": "Checks for due calendar reminders every minute",
		"dag_config": map[string]interface{}{
			"dag_id":      "calendar_reminders_check",
			"name":        "Calendar: Reminders Check",
			"description": "Calls the calendar app reminder check endpoint",
			"nodes": []map[string]interface{}{
				{
					"node_id":            "check_reminders",
					"node_type":          "http-api",
					"execution_strategy": "raft-leader",
					"metadata": map[string]interface{}{
						"url":             appURL + "/api/reminders/check",
						"method":          "POST",
						"timeout_ms":      10000,
						"max_retries":     1,
						"expected_status": 200,
					},
				},
			},
		},
		"trigger_type": "periodic",
		"trigger_config": map[string]interface{}{
			"periodic": map[string]interface{}{
				"schedule":    "* * * * *",
				"timezone":    "Local",
				"max_retries": 1,
			},
		},
		"is_enabled": true,
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", coreURL+"/apps/automation/api/automations", bytes.NewReader(b))
	if err != nil {
		log.Printf("⚠️  Failed to create reminder automation request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️  Failed to register reminder automation: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("✅ Registered calendar reminder automation (every minute)")
	} else {
		log.Printf("⚠️  Calendar reminder automation registration returned %d", resp.StatusCode)
	}
}

func automationExists(coreURL, token, name string) bool {
	req, err := http.NewRequest("GET", coreURL+"/apps/automation/api/automations", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		Automations []struct {
			Name string `json:"name"`
		} `json:"automations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	for _, a := range result.Automations {
		if a.Name == name {
			return true
		}
	}
	return false
}
