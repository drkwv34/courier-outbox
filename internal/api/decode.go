package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

const maxJSONBody = 256 * 1024

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &domain.ValidationError{Field: "body", Reason: "unexpected trailing data"}
	}
	return nil
}

func isUnknownJSONField(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "json: unknown field")
}
