package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/NYTimes/gziphandler"
)

var (
	//go:embed webapp
	webappFS embed.FS
	assetsFS fs.FS
)

func init() {
	var err error
	assetsFS, err = fs.Sub(webappFS, "webapp/assets")
	if err != nil {
		panic(err)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", gziphandler.GzipHandler(http.FileServer(http.FS(assetsFS)))))
	mux.HandleFunc("/", handleIndexPage())

	mux.Handle("/go/version", handleVersion())
	mux.Handle("/go/run", gziphandler.GzipHandler(handleRun()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	err := http.ListenAndServe(":"+port, http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		log.Println(req.URL)
		mux.ServeHTTP(res, req)
	}))
	if err != nil {
		panic(err)
	}
}

func handleIndexPage() func(res http.ResponseWriter, req *http.Request) {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.Error(res, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		indexHTML, err := webappFS.ReadFile("webapp/index.html")
		if err != nil {
			log.Println("failed to open index file", err)
			http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		res.Header().Set("cache-control", "no-cache")
		res.Header().Set("content-type", "text/html")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write(indexHTML)
	}
}
