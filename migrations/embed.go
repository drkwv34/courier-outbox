// Package migrations embeds the forward-only SQL migrations so the courier
// binary can apply its own schema (ADR 0003).
package migrations

import "embed"

// FS holds every NNNNNN_*.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
