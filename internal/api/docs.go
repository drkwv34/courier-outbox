package api

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed docsui/*
var docsUI embed.FS

func mountDocs(r chi.Router, logger *slog.Logger) {
	sub, err := fs.Sub(docsUI, "docsui")
	if err != nil {
		logger.Error("docs ui embed", slog.Any("err", err))
		return
	}
	r.Get("/docs", serveDocsIndex(logger))
	r.Handle("/docs/*", http.StripPrefix("/docs/", http.FileServer(http.FS(sub))))
}

func serveDocsIndex(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		body, err := docsUI.ReadFile("docsui/index.html")
		if err != nil {
			writeError(w, logger, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			logger.Warn("write docs", slog.Any("err", err))
		}
	}
}
