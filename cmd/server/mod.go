package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"golang.org/x/tools/txtar"
)

func handleModTidy(goExecPath string) http.HandlerFunc {
	env := mergeEnv(os.Environ(), goEnvOverride()...)

	return func(res http.ResponseWriter, req *http.Request) {
		const maxReadBytes = (1 << 10) * 8
		_ = req.ParseMultipartForm(maxReadBytes)
		defer closeAndIgnoreError(req.Body)
		archive, err := readArchive(req.Form)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		tmp, err := writeDirectory(archive)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		var outputBuffer bytes.Buffer

		ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
		defer cancel()

		get := exec.CommandContext(ctx, goExecPath,
			"go", "get", "-u", "./...",
		)
		get.Stdout = &outputBuffer
		get.Stderr = &outputBuffer
		get.Env = env
		get.Dir = tmp
		outputBuffer.WriteString("$ " + strings.Join(append([]string{path.Base(get.Path)}, get.Args...), " "))
		err = get.Run()
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		tidy := exec.CommandContext(ctx, goExecPath,
			"mod", "tidy",
		)
		tidy.Stdout = &outputBuffer
		tidy.Stderr = &outputBuffer
		tidy.Env = env
		tidy.Dir = tmp
		outputBuffer.WriteString("$ " + strings.Join(append([]string{path.Base(tidy.Path)}, tidy.Args...), " "))
		err = tidy.Run()
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		if err := readDirectory(tmp, archive); err != nil {
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
