package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

type (
	Run struct {
		Location           string
		RunID              int
		BinaryBase64       string
		SourceHTMLDocument string
		WASMExecJS         template.JS
	}
	RunFailure struct {
		BuildLogs string
		RunID     int
	}
)

func (dir *FilesystemDirectory) buildWASM(ctx context.Context, env []string, goExecPath string) (string, error) {
	var outputBuffer bytes.Buffer
	const output = "main.wasm"
	buildArgs := []string{
		"build",
		"-o", output,
		fmt.Sprintf("-gcflags=-trimpath=%s", dir.TempDir),
		fmt.Sprintf("-asmflags=-trimpath=%s", dir.TempDir),
	}
	err := dir.execGo(ctx, env, goExecPath, buildArgs...)
	if err != nil {
		return "", errors.New(outputBuffer.String())
	}
	wasmBuild, err := os.ReadFile(filepath.Join(dir.TempDir, output))
	if err != nil {
		return "", fmt.Errorf("failed to open build file: %w", err)
	}
	encodedBuild := base64.StdEncoding.EncodeToString(wasmBuild)
	return encodedBuild, nil
}

func handleDownload(res http.ResponseWriter, req *http.Request) {
	archive, err := newRequestArchive(req)
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	archive.ServeHTTP(res, req)
}

func handleRun(goExecPath string) http.HandlerFunc {
	env := mergeEnv(os.Environ(), goEnvOverride()...)

	wasmExecJS, err := fs.ReadFile(assets, "assets/lib/wasm_exec.js")
	if err != nil {
		fmt.Println(err)
		fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() || err != nil {
				return err
			}
			fmt.Println("found: ", path)
			return nil
		})
		os.Exit(0)
	}

	return func(res http.ResponseWriter, req *http.Request) {
		var runID = 1
		if runIDQuery := req.FormValue("run-id"); runIDQuery != "" {
			var err error
			runID, err = strconv.Atoi(runIDQuery)
			if err != nil {
				http.Error(res, "invalid run id", http.StatusBadRequest)
				return
			}
		}

		ctx, cancel := context.WithTimeout(req.Context(), time.Second*30)
		defer cancel()

		currentURL, err := url.Parse(req.Header.Get("hx-current-url"))
		if err != nil {
			log.Println("failed to parse current url", err)
			http.Error(res, "failed to parse current url", http.StatusInternalServerError)
			return
		}
		if !slices.Contains([]string{"http", "https"}, currentURL.Scheme) {
			http.Error(res, "unsupported scheme", http.StatusBadRequest)
			return
		}

		dir, err := newRequestDirectory(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		defer func() {
			_ = dir.close()
		}()

		buildBase64, err := dir.buildWASM(ctx, env, goExecPath)
		if err != nil {
			renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
				return templates.ExecuteTemplate(w, "build-failure", RunFailure{
					RunID:     runID,
					BuildLogs: err.Error(),
				})
			})
			return
		}

		data := Run{
			Location:     fmt.Sprintf("%s://%s", currentURL.Scheme, currentURL.Host),
			RunID:        runID,
			BinaryBase64: buildBase64,
			WASMExecJS:   template.JS(wasmExecJS),
		}

		if req.Header.Get("HX-Target") == "runner" {
			var buf bytes.Buffer
			if err := templates.ExecuteTemplate(&buf, "run.html.template", data); err != nil {
				log.Println("failed to execute index template", err)
				http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			data.SourceHTMLDocument = buf.String()
			renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
				return templates.ExecuteTemplate(w, "run-item", data)
			})
		} else {
			renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
				return templates.ExecuteTemplate(w, "run.html.template", data)
			})
		}
	}
}
