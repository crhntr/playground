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
	mux.Handle("POST /fmt", handleFmt())
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

//func renderHTML(res http.ResponseWriter, _ *http.Request, ts *template.Template, code int, data any) {
//	var buf bytes.Buffer
//	if err := ts.Execute(&buf, data); err != nil {
//		log.Println("failed to execute index template", err)
//		http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
//		return
//	}
//	res.Header().Set("x-template-name", ts.Name())
//	res.Header().Set("content-type", "text/html")
//	res.Header().Set("content-length", strconv.Itoa(buf.Len()))
//	res.WriteHeader(code)
//	_, _ = res.Write(buf.Bytes())
//}

func closeAndIgnoreError(c io.Closer) { _ = c.Close() }

func removeZeros[T comparable](in []T) []T {
	filtered := in[:0]
	for _, p := range in {
		var zero T
		if p == zero {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

func handleFmt() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		archive, err := newRequestArchive(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		if err := archive.fmt(); err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", archive)
		})
	}
}

func renderHTML(res http.ResponseWriter, _ *http.Request, status int, execute func(w io.Writer) error) {
	var buf bytes.Buffer
	if err := execute(&buf); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(res, status, "text/html; charset=utf-8", buf.Bytes())
}

//func renderJSON(status int, res http.ResponseWriter, data any) {
//	buf, err := json.MarshalIndent(data, "", "\t")
//	if err != nil {
//		http.Error(res, err.Error(), http.StatusBadRequest)
//		return
//	}
//	writeResponse(res, status, "application/json; charset=utf-8", buf)
//}

func writeResponse(res http.ResponseWriter, code int, contentType string, buf []byte) {
	h := res.Header()
	h.Set("content-type", contentType)
	h.Set("content-length", strconv.Itoa(len(buf)))
	h.Set("cache-control", "no-cache")
	res.WriteHeader(code)
	_, _ = res.Write(buf)
}
