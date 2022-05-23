package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/NYTimes/gziphandler"
	domAST "github.com/crhntr/window/ast"

	"github.com/crhntr/playground/view"
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
		template, err := webappFS.Open("webapp/index.html")
		if err != nil {
			log.Println("failed to open index file", err)
			http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		doc, err := domAST.ParseDocument(template)
		if err != nil {
			log.Println("failed to open index file", err)
			http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		view.IndexData{
			GoVersion: string(readGoVersion(context.Background())),
			Copyright: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
		}.Update(doc.Body())

		res.Header().Set("cache-control", "no-cache")
		res.Header().Set("content-type", "text/html")
		res.WriteHeader(http.StatusOK)
		if err := domAST.RenderDocument(res, doc); err != nil {
			return
		}
	}
}
