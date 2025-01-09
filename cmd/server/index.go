package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
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

func handleIndexPage(goVersion string, examples []Example) http.HandlerFunc {
	const defaultExampleName = "hello-world"
	return func(res http.ResponseWriter, req *http.Request) {
		data := Index{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       goVersion,
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

func handleGETInstall(goVersion string) http.HandlerFunc {
	type Data struct {
		GoVersion       string
		CopyrightNotice string
	}

	return func(res http.ResponseWriter, req *http.Request) {
		data := Data{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       goVersion,
		}
		renderHTML(res, req, templates.Lookup("upload.html.template"), http.StatusOK, data)
	}
}

func handlePOSTInstall(goVersion string, examples []Example) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		var zipBuffer []byte
		defer closeAndIgnoreError(req.Body)
		if pr, err := req.MultipartReader(); err == nil {
			for {
				part, err := pr.NextPart()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					http.Error(res, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return
				}
				if part.FormName() != "zip" {
					http.Error(res, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return
				}
				buf, err := io.ReadAll(part)
				if err != nil {
					http.Error(res, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return
				}
				zipBuffer = buf
				break
			}
		} else {
			const maxReadBytes = (1 << 10) * 8
			body := io.LimitReader(req.Body, maxReadBytes)
			defer closeAndIgnoreError(req.Body)
			buf, err := io.ReadAll(body)
			if err != nil {
				http.Error(res, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			zipBuffer = buf
		}

		zr, err := zip.NewReader(bytes.NewReader(zipBuffer), int64(len(zipBuffer)))
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		var archive txtar.Archive
		err = fs.WalkDir(zr, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !isPermittedFile(p) {
				return nil
			}
			fileBuf, err := fs.ReadFile(zr, p)
			if err != nil {
				return err
			}
			archive.Files = append(archive.Files, txtar.File{
				Name: path.Clean(p),
				Data: fileBuf,
			})
			return nil
		})
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		data := Index{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       goVersion,
			Examples:        slices.Clone(examples),
			Name:            "Upload",
			Archive:         &archive,
		}
		renderHTML(res, req, templates.Lookup("index.html.template"), http.StatusOK, data)
	}
}

func isPermittedFile(in string) bool {
	if len(in) > 200 || in == "" {
		return false
	}
	base := path.Base(in)
	if strings.HasPrefix(base, ".") {
		return false
	}
	switch base {
	case "LICENSE", "go.sum":
		return true
	}
	switch strings.ToLower(path.Ext(in)) {
	case ".go", ".mod", ".html", ".gohtml", ".md", ".txt", ".json", ".yml", ".yaml", ".tmpl", ".css":
		return true
	}
	return false
}
