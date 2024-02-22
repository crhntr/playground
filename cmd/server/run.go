package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func handleRun(ts *template.Template) http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	wasmExecJS, err := fs.ReadFile(assets, "assets/wasm_exec.js")
	if err != nil {
		log.Fatal(err)
	}

	return func(res http.ResponseWriter, req *http.Request) {
		req.ParseMultipartForm((1 << 10) * 8)
		mainGo := req.FormValue("main.go")

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

		tmp, err := os.MkdirTemp("", "")
		if err != nil {
			log.Println("failed to create temporary directory", err)
			http.Error(res, "failed to create temporary directory", http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = os.RemoveAll(tmp)
		}()

		defer func() {
			_ = req.Body.Close()
		}()

		err = createMainFile(tmp, mainGo)
		if err != nil {
			log.Println("failed to write main.go", err)
			http.Error(res, "failed to write main.go", http.StatusInternalServerError)
			return
		}
		err = createModFile(tmp)
		if err != nil {
			log.Println("failed to create go.mod", err)
			http.Error(res, "failed to create go.mod", http.StatusInternalServerError)
			return
		}

		const output = "main.wasm"
		cmd := exec.CommandContext(ctx, goExecPath,
			"build",
			"-o", output,
			fmt.Sprintf("-gcflags=-trimpath=%s", tmp),
			fmt.Sprintf("-asmflags=-trimpath=%s", tmp),
		)
		var outputBuffer bytes.Buffer
		cmd.Stdout = &outputBuffer
		cmd.Stderr = &outputBuffer
		cmd.Env = env
		cmd.Dir = tmp
		err = cmd.Run()
		if err != nil {
			if req.Header.Get("HX-Target") == "runner" {
				renderHTML(res, req, ts, "build-failure", http.StatusOK, struct {
					BuildLogs string
					RunID     int
				}{
					BuildLogs: outputBuffer.String(),
					RunID:     runID,
				})
			} else {
				http.Error(res, outputBuffer.String(), http.StatusBadRequest)
			}
			return
		}

		mainWASM, err := os.ReadFile(filepath.Join(tmp, output))
		if err != nil {
			log.Println("failed to open build file", err)
			http.Error(res, "failed to open build file", http.StatusInternalServerError)
			return
		}

		hxCurrentURL := req.Header.Get("hx-current-url")
		if hxCurrentURL == "" {
			hxCurrentURL = "http://" + req.Host
		}
		currentURL, err := url.Parse(hxCurrentURL)
		if err != nil {
			log.Println("failed to parse current url", err)
			http.Error(res, "failed to parse current url", http.StatusInternalServerError)
			return
		}

		data := struct {
			Location           string
			RunID              int
			BinaryBase64       string
			SourceHTMLDocument string
			WASMExecJS         template.JS
		}{
			Location:     fmt.Sprintf("%s://%s", currentURL.Scheme, currentURL.Host),
			RunID:        runID,
			BinaryBase64: base64.StdEncoding.EncodeToString(mainWASM),
			WASMExecJS:   template.JS(wasmExecJS),
		}

		if req.Header.Get("HX-Target") == "runner" {
			var buf bytes.Buffer
			if err := ts.ExecuteTemplate(&buf, "run.html.template", data); err != nil {
				log.Println("failed to execute index template", err)
				http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			data.SourceHTMLDocument = buf.String()
			renderHTML(res, req, ts, "run-item", http.StatusOK, data)
		} else {
			renderHTML(res, req, ts, "run.html.template", http.StatusOK, data)
		}
	}
}

func createMainFile(dir, mainGo string) error {
	fp := filepath.Join(dir, "main.go")
	return os.WriteFile(fp, []byte(mainGo), 0644)
}

func createModFile(dir string) error {
	fp := filepath.Join(dir, "go.mod")
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = f.Write([]byte("module playground\n"))
	if err != nil {
		return err
	}
	return nil
}
