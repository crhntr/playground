package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	//go:embed assets
	assets embed.FS

	//go:embed templates
	templates embed.FS
)

func main() {
	ts := template.Must(template.New("").ParseFS(templates, "templates/*.template"))

	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("GET /", handleIndexPage(ts))

	mux.Handle("GET /go/version", handleVersion())
	mux.Handle("POST /go/run", handleRun(ts))

	port, ok := os.LookupEnv("PORT")
	if !ok || port == "" {
		port = "8080"
	}
	err := http.ListenAndServe(":"+port, http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		log.Println(req.Method, req.URL)
		mux.ServeHTTP(res, req)
	}))
	if err != nil {
		panic(err)
	}
}

type Index struct {
	CopyrightNotice, GoVersion string
	Examples                   []Example
	Name                       string
	MainGo                     string
}

func handleIndexPage(ts *template.Template) func(res http.ResponseWriter, req *http.Request) {
	const defaultExampleName = "hello-world"

	exampleDirectories, err := fs.ReadDir(assets, "assets/examples")
	if err != nil {
		log.Fatal(err)
	}
	examples := make([]Example, 0, len(exampleDirectories))
	var defaultExampleMainGo string
	for _, dir := range exampleDirectories {
		examples = append(examples, Example{Name: dir.Name()})

		if dir.Name() == defaultExampleName {
			buf, err := fs.ReadFile(assets, path.Join("assets/examples", defaultExampleName, "main.go"))
			if err != nil {
				log.Fatal(err)
			}
			defaultExampleMainGo = string(buf)
		}
	}
	if defaultExampleMainGo == "" {
		log.Fatalf("failed to read main.go for default example %q", defaultExampleName)
	}

	return func(res http.ResponseWriter, req *http.Request) {
		data := Index{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       string(readGoVersion(req.Context())),
			Examples:        slices.Clone(examples),
			MainGo:          defaultExampleMainGo,
			Name:            defaultExampleName,
		}

		if q := req.URL.Query(); q.Has("example") {
			exampleQuery := strings.TrimSpace(q.Get("example"))
			if index := slices.IndexFunc(data.Examples, func(e Example) bool {
				return e.Name == exampleQuery
			}); index >= 0 && index < len(data.Examples) {
				buf, err := fs.ReadFile(assets, path.Join("assets/examples", exampleQuery, "main.go"))
				if err != nil {
					log.Println("failed to read example", err)
					http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				data.MainGo = string(buf)
				data.Name = exampleQuery
			}
		}

		renderHTML(res, req, ts, "index.html.template", http.StatusOK, data)
	}
}

type Example struct {
	Name string
}

func renderHTML(res http.ResponseWriter, _ *http.Request, ts *template.Template, name string, code int, data any) {
	var buf bytes.Buffer
	if err := ts.ExecuteTemplate(&buf, name, data); err != nil {
		log.Println("failed to execute index template", err)
		http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	res.Header().Set("x-template-name", name)
	res.Header().Set("content-type", "text/html")
	res.Header().Set("content-length", strconv.Itoa(buf.Len()))
	res.WriteHeader(code)
	_, _ = res.Write(buf.Bytes())
}
