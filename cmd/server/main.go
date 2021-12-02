package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", http.FileServer(http.FS(assetsFS))))
	mux.Handle("/", http.HandlerFunc(handlePage))

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
