package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSON_RoundTripContainsLivePaths(t *testing.T) {
	t.Parallel()

	raw, err := JSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("openapi = %v, want 3.0.3", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths type %T", doc["paths"])
	}
	for _, p := range []string{
		"/healthz",
		"/readyz",
		"/openapi.json",
		"/metrics",
		"/v1/subscriptions",
		"/v1/events",
		"/v1/deliveries",
	} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s", p)
		}
	}
}

func TestYAMLToJSON_Invalid(t *testing.T) {
	t.Parallel()

	_, err := yamlToJSON([]byte(":\n  - not: valid: yaml"))
	if err == nil || !strings.Contains(err.Error(), "parse yaml") {
		t.Fatalf("err = %v, want parse yaml", err)
	}
	_, err = yamlToJSON(nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty err = %v", err)
	}
}
