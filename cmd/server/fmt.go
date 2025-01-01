package main

import (
	"net/http"

	"github.com/crhntr/txtarfmt"
	"golang.org/x/tools/txtar"
)

func handleFmt() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		const maxReadBytes = (1 << 10) * 8
		_ = req.ParseMultipartForm(maxReadBytes)
		defer closeAndIgnoreError(req.Body)
		archive, err := readArchive(req.Form)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if err := txtarfmt.Archive(archive, txtarfmt.Configuration{}); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		renderHTML(res, req, templates.Lookup("editor"), http.StatusOK, struct {
			Archive *txtar.Archive
		}{
			Archive: archive,
		})
	}
}
