package main

import (
	"io"
	"net/http"
	"path"
	"slices"

	"golang.org/x/tools/txtar"
)

func handleNewFile() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		dir, err := readMemoryDirectory(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		filename := req.FormValue("new-filename")
		if filename == "" {
			http.Error(res, "filename required", http.StatusBadRequest)
			return
		}
		if !isPermittedFile(filename) {
			http.Error(res, "file type not permitted", http.StatusBadRequest)
			return
		}

		for _, f := range dir.Archive.Files {
			if f.Name == filename {
				http.Error(res, "file already exists", http.StatusBadRequest)
				return
			}
		}

		var content []byte
		if path.Ext(filename) == ".go" {
			content = []byte("package main\n")
		}

		dir.Archive.Files = append(dir.Archive.Files, txtar.File{
			Name: filename,
			Data: content,
		})
		dir.ActiveFile = filename
		dir.OpenFiles = append(dir.OpenFiles, filename)
		dir.normalizeIDEState()

		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", dir)
		})
	}
}

func handleDeleteFile() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		dir, err := readMemoryDirectory(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		filename := req.FormValue("delete-filename")
		if filename == "" {
			http.Error(res, "filename required", http.StatusBadRequest)
			return
		}

		dir.Archive.Files = slices.DeleteFunc(dir.Archive.Files, func(f txtar.File) bool {
			return f.Name == filename
		})
		if dir.ActiveFile == filename {
			dir.ActiveFile = ""
		}
		dir.OpenFiles = slices.DeleteFunc(dir.OpenFiles, func(s string) bool { return s == filename })
		dir.normalizeIDEState()

		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", dir)
		})
	}
}
