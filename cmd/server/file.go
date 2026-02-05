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
		archive, err := newRequestArchive(req)
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

		// Check if file already exists
		for _, f := range archive.Archive.Files {
			if f.Name == filename {
				http.Error(res, "file already exists", http.StatusBadRequest)
				return
			}
		}

		var content []byte
		if path.Ext(filename) == ".go" {
			content = []byte("package main\n")
		}

		archive.Archive.Files = append(archive.Archive.Files, txtar.File{
			Name: filename,
			Data: content,
		})

		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", archive)
		})
	}
}

func handleDeleteFile() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		archive, err := newRequestArchive(req)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		filename := req.FormValue("delete-filename")
		if filename == "" {
			http.Error(res, "filename required", http.StatusBadRequest)
			return
		}

		archive.Archive.Files = slices.DeleteFunc(archive.Archive.Files, func(f txtar.File) bool {
			return f.Name == filename
		})

		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "editor", archive)
		})
	}
}
