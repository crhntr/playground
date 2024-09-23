package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"
)

type Index struct {
	CopyrightNotice, GoVersion string
	Examples                   []Example
	Name                       string
	MainGo                     string
}

type Example struct {
	Name string
}

func handleIndexPage() func(res http.ResponseWriter, req *http.Request) {
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

	goVersion, err := readGoVersion(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	return func(res http.ResponseWriter, req *http.Request) {
		data := Index{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       string(goVersion),
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

		renderHTML(res, req, templates.Lookup("index.html.template"), http.StatusOK, data)
	}
}
