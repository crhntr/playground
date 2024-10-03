package main

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

var (
	//go:embed assets
	assets embed.FS
)

func main() {
	gv, err := readGoVersion(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	goVersion := string(gv)
	exampleDirectories, err := fs.Glob(assets, "assets/examples/*.txtar")
	if err != nil {
		log.Fatal(err)
	}
	examples := make([]Example, 0, len(exampleDirectories))
	for _, dir := range exampleDirectories {
		examples = append(examples, Example{Name: strings.TrimSuffix(path.Base(dir), ".txtar")})
	}
	goExecPath, err := exec.LookPath("go")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.FileServer(http.FS(assets)))
	mux.Handle("GET /", handleIndexPage(goVersion, examples))

	mux.Handle("GET /go/version", handleVersion(goVersion))
	mux.Handle("POST /go/run", handleRun(goExecPath))
	mux.Handle("POST /go/mod/tidy", handleModTidy(goExecPath))
	mux.HandleFunc("POST /download", handleDownload)

	mux.HandleFunc("GET /upload", handleGETInstall(goVersion))
	mux.HandleFunc("POST /upload", handlePOSTInstall(goVersion, examples))

	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

var (
	//go:embed templates
	templateSource embed.FS

	templates = template.Must(template.New("").Funcs(template.FuncMap{
		"bytesToString": func(in []byte) string { return string(in) },
	}).ParseFS(templateSource, "templates/*.template"))
)

func renderHTML(res http.ResponseWriter, _ *http.Request, ts *template.Template, code int, data any) {
	var buf bytes.Buffer
	if err := ts.Execute(&buf, data); err != nil {
		log.Println("failed to execute index template", err)
		http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	res.Header().Set("x-template-name", ts.Name())
	res.Header().Set("content-type", "text/html")
	res.Header().Set("content-length", strconv.Itoa(buf.Len()))
	res.WriteHeader(code)
	_, _ = res.Write(buf.Bytes())
}

func closeAndIgnoreError(c io.Closer) { _ = c.Close() }
