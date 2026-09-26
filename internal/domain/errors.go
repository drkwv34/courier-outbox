package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors. Adapters translate driver errors into these; the api
// layer is the only place that maps them to HTTP status codes.
var (
	// ErrNotFound means the resource does not exist or belongs to another
	// API key. The two cases are deliberately indistinguishable.
	ErrNotFound = errors.New("not found")
	// ErrConflict means a uniqueness invariant would be violated.
	ErrConflict = errors.New("conflict")
	// ErrValidation means caller-supplied input is malformed.
	ErrValidation = errors.New("validation failed")
)

// ValidationError carries the field that failed validation. It unwraps to
// ErrValidation.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

// Unwrap lets errors.Is(err, ErrValidation) match.
func (e *ValidationError) Unwrap() error { return ErrValidation }
