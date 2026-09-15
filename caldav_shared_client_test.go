package calendar

import (
	"testing"
	"time"
)

// The CalDAV wrapper is per-calendar (URL + creds), but its inner *http.Client
// used to be a fresh transport per calendar per sync. Both constructors now share
// one pooled client. Assert every wrapper points at the same shared instance.
func TestCalDAVClientsShareOnePool(t *testing.T) {
	if caldavHTTPClient == nil {
		t.Fatal("expected a shared CalDAV http client")
	}
	if caldavHTTPClient.Timeout <= 0 || caldavHTTPClient.Timeout > time.Minute {
		t.Fatalf("shared CalDAV client timeout = %v, want a sane positive bound", caldavHTTPClient.Timeout)
	}
	a := NewCalDAVClient("https://dav.example.com", "u", "p")
	b := NewCalDAVClientWithBearer("https://dav.example.com", "tok")
	if a.client != caldavHTTPClient || b.client != caldavHTTPClient {
		t.Error("CalDAV wrappers must reuse the single shared pooled client")
	}
	if a.client != b.client {
		t.Error("both CalDAV constructors must share the same inner client")
	}
}
