package calendar

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	client "github.com/localitas/localitas-go"
)

// The doc the app serves at /swagger.json is the spec core's tool index
// reads: every key one client.APISpec has, written back byte for byte, and
// every JSON example a JSON document.
func TestSwagger_ServesTheSpecCoreReads(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleSwagger(rec, httptest.NewRequest(http.MethodGet, "/swagger.json", nil))
	var spec client.APISpec
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		t.Fatalf("the served doc does not read as client.APISpec: %v", err)
	}
	again, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(rec.Body.String()) != string(again) {
		t.Fatalf("the served doc\n%s\nreads back as\n%s", rec.Body.String(), again)
	}
	for _, ep := range spec.Endpoints {
		for _, body := range []*client.APIBody{ep.RequestBody, ep.Response} {
			if body != nil && body.ContentType == "application/json" && !json.Valid([]byte(body.Example)) {
				t.Errorf("%s %s: example %q is not JSON", ep.Method, ep.Path, body.Example)
			}
		}
	}
}
