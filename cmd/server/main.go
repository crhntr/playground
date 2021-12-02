package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", http.FileServer(http.FS(assetsFS))))
	mux.Handle("/", http.HandlerFunc(handlePage))

	mux.Handle("/go/version", handleVersion())
	mux.Handle("/go/env", handleEnv())

	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		panic(err)
	}
}

var (
	//go:embed webapp
	webappFS embed.FS
	assetsFS fs.FS

	pages = map[string]string{
		"/": "webapp/index.gohtml",
	}
)

func init() {
	var err error
	assetsFS, err = fs.Sub(webappFS, "webapp/assets")
	if err != nil {
		panic(err)
	}
}

func handlePage(res http.ResponseWriter, req *http.Request) {
	page, ok := pages[req.URL.Path]
	if !ok {
		res.WriteHeader(http.StatusNotFound)
		return
	}
	templates, err := template.ParseFS(webappFS, page)
	if err != nil {
		log.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	res.Header().Set("content-type", "text/html")
	res.WriteHeader(http.StatusOK)
	if err := templates.Execute(res, struct{}{}); err != nil {
		return
	}
}

func handleVersion() http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second*2)
		defer cancel()

		cmd := exec.CommandContext(ctx, goExecPath, "version")
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmd.Env = env
		err := cmd.Run()
		if err != nil {
			http.Error(res, buf.String(), http.StatusInternalServerError)
			return
		}
		res.WriteHeader(http.StatusOK)
		_, _ = io.Copy(res, &buf)
	}
}

func handleEnv() http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)
	fmt.Println(env)

	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second*2)
		defer cancel()

		cmd := exec.CommandContext(ctx, goExecPath, "env")
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmd.Env = env
		err := cmd.Run()
		if err != nil {
			http.Error(res, buf.String(), http.StatusInternalServerError)
			return
		}
		res.WriteHeader(http.StatusOK)
		_, _ = io.Copy(res, &buf)
	}
}

func goEnvOverride() []string { return []string{"GOOS=js", "GOARCH=wasm"} }

func mergeEnv(env []string, additional ...string) []string {
	l := len(env) + len(env)
	m := make(map[string]string, l)
	keys := make([]string, 0, l)
	for _, v := range append(additional, env...) {
		x := strings.Index(v, "=")
		k := v[:x]
		keys = append(keys, k)
		m[k] = v[x+1:]
	}
	result := make([]string, 0, l)
	for _, k := range keys {
		result = append(result, k+"="+m[k])
	}
	return result
}
