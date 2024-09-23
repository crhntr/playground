package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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

func buildWASM(ctx context.Context, env []string, goExecPath, mainGo string) (string, error) {
	tmp, err := os.MkdirTemp("", "")
	if err != nil {
		log.Println("failed to create temporary directory", err)
		return "", fmt.Errorf("failed to create temporary directory")
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()

	if err := checkImports(mainGo); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(mainGo), 0644); err != nil {
		return "", fmt.Errorf("failed to write main.go: %w", err)
	}
	if err = os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module playground\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to create go.mod: %w", err)
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
		return "", errors.New(outputBuffer.String())
	}

	wasmBuild, err := os.ReadFile(filepath.Join(tmp, output))
	if err != nil {
		return "", fmt.Errorf("failed to open build file: %w", err)
	}
	encodedBuild := base64.StdEncoding.EncodeToString(wasmBuild)
	return encodedBuild, nil
}

func handleRun() http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		log.Fatal(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	wasmExecJS, err := fs.ReadFile(assets, "assets/lib/wasm_exec.js")
	if err != nil {
		log.Fatal(err)
	}

	return func(res http.ResponseWriter, req *http.Request) {
		const maxReadBytes = (1 << 10) * 8
		req.ParseMultipartForm(maxReadBytes)
		defer closeAndIgnoreError(req.Body)
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

		currentURL, err := url.Parse(req.Header.Get("hx-current-url"))
		if err != nil {
			log.Println("failed to parse current url", err)
			http.Error(res, "failed to parse current url", http.StatusInternalServerError)
			return
		}

		buildBase64, err := buildWASM(ctx, env, goExecPath, mainGo)
		if err != nil {
			renderHTML(res, req, templates.Lookup("run-item"), http.StatusOK, RunFailure{
				RunID:     runID,
				BuildLogs: err.Error(),
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
			renderHTML(res, req, templates.Lookup("run-item"), http.StatusOK, data)
		} else {
			renderHTML(res, req, templates.Lookup("run.html.template"), http.StatusOK, data)
		}
	}
}

func checkImports(mainGo string) error {
	var fileSet token.FileSet
	file, err := parser.ParseFile(&fileSet, "main.go", mainGo, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("failed to parse main.go: %w", err)
	}

	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if slices.Index(permittedPackages(), path) >= 0 {
			continue
		}
		return fmt.Errorf("importing %q is not permitted on this site", path)
	}

	return nil
}

//go:embed assets/import_allow_list.txt
var permittedPackagesString string

func permittedPackages() []string {
	list := strings.Split(permittedPackagesString, "\n")
	return list
}
