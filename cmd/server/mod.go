package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"
)

func handleModTidy(goExecPath string) http.HandlerFunc {
	env := mergeEnv(os.Environ(), goEnvOverride()...)

	return func(res http.ResponseWriter, req *http.Request) {
		dir, err := newRequestDirectory(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()

		if err := dir.execGo(ctx, env, goExecPath, "mod", "tidy"); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if err := dir.readFiles(); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", dir)
		})
	}
}
