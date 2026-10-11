// Package openapi embeds the committed OpenAPI 3 YAML document and converts
// it to JSON for GET /openapi.json.
package openapi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// YAML is the committed OpenAPI 3 document (source of truth).
//
//go:embed openapi.yaml
var YAML []byte

// JSON returns the OpenAPI document as JSON. The YAML is the single source
// of truth; this conversion happens once at process start.
func JSON() ([]byte, error) {
	return yamlToJSON(YAML)
}

func yamlToJSON(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("openapi: document is empty")
	}
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("openapi: parse yaml: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("openapi: encode json: %w", err)
	}
	return out, nil
}
