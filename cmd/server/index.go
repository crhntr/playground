package main

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/txtar"
)

type Index struct {
	CopyrightNotice, GoVersion string
	Examples                   []Example
	Name                       string
	Archive                    *txtar.Archive
}

type Example struct {
	Name string
}

func handleIndexPage() func(res http.ResponseWriter, req *http.Request) {
	const defaultExampleName = "hello-world"

	exampleDirectories, err := fs.Glob(assets, "assets/examples/*.txtar")
	if err != nil {
		log.Fatal(err)
	}
	examples := make([]Example, 0, len(exampleDirectories))
	for _, dir := range exampleDirectories {
		examples = append(examples, Example{Name: strings.TrimSuffix(path.Base(dir), ".txtar")})
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
			Name:            defaultExampleName,
		}

		if q := req.URL.Query(); q.Has("example") {
			data.Name = cmp.Or(strings.TrimSpace(q.Get("example")), defaultExampleName)
		}
		if data.Name != "" {
			if index := slices.IndexFunc(data.Examples, func(e Example) bool {
				return e.Name == data.Name
			}); index >= 0 && index < len(data.Examples) {
				buf, err := fs.ReadFile(assets, path.Join("assets/examples", data.Name+".txtar"))
				if err != nil {
					log.Println("failed to read example", err)
					http.Error(res, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				data.Archive = txtar.Parse(buf)
			}
		}
		renderHTML(res, req, templates.Lookup("index.html.template"), http.StatusOK, data)
	}
}
