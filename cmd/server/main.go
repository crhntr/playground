package main

import (
	"bytes"
	"cmp"
	"embed"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

var (
	//go:embed assets
	assets embed.FS
)

func main() {
	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("GET /", handleIndexPage())

	mux.Handle("GET /go/version", handleVersion())
	mux.Handle("POST /go/run", handleRun())

	addr := ":" + cmp.Or(os.Getenv("PORT"), "8080")
	err := http.ListenAndServe(addr, mux)
	if err != nil {
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
