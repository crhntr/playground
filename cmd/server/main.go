package main

import (
	"bytes"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
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
