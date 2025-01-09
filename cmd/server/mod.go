package main

import (
	"bytes"
	"context"
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

		ctx := req.Context()
		var outputBuffer bytes.Buffer

		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()

		if err := dir.execGo(ctx, env, &outputBuffer, goExecPath, "mod", "tidy"); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if err := dir.readFiles(); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		renderHTML(res, req, templates.Lookup("editor"), http.StatusOK, dir)
	}
}
