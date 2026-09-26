package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// readinessTimeout bounds each dependency ping so a hung dependency cannot
// stall the probe past typical orchestrator timeouts.
const readinessTimeout = 2 * time.Second

// ReadinessCheck is one dependency probed by /readyz.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// readyz reports whether every dependency answers. Failures name the
// dependency only; driver errors go to the log.
func readyz(logger *slog.Logger, checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		failed := make([]bool, len(checks))
		var wg sync.WaitGroup
		for i, c := range checks {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
				defer cancel()
				if err := c.Check(ctx); err != nil {
					failed[i] = true
					logger.WarnContext(r.Context(), "readiness check failed",
						slog.String("request_id", middleware.GetReqID(r.Context())),
						slog.String("dependency", c.Name),
						slog.Any("err", err))
				}
			}()
		}
		wg.Wait()

		var down []string
		for i, f := range failed {
			if f {
				down = append(down, checks[i].Name)
			}
		}
		if len(down) > 0 {
			writeError(w, logger, http.StatusServiceUnavailable, "unavailable", "not ready: "+strings.Join(down, ", "))
			return
		}
		writeJSON(w, logger, http.StatusOK, map[string]string{"status": "ready"})
	}
}
