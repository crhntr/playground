package main

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v80/github"
	"golang.org/x/time/rate"
	"golang.org/x/tools/txtar"
)

func newGitHubClient() *github.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return github.NewClient(nil)
	}
	return github.NewClient(nil).WithAuthToken(token)
}

func newGistRateLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Every(time.Second), 5)
}

func handleGist(goVersion string, examples []Example, goExecPath string, ghClient *github.Client, limiter *rate.Limiter) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		gistID := req.PathValue("gistID")
		if gistID == "" {
			http.Error(res, "missing gist ID", http.StatusBadRequest)
			return
		}

		if !limiter.Allow() {
			http.Error(res, "rate limit exceeded, try again later", http.StatusTooManyRequests)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
		defer cancel()

		gist, resp, err := ghClient.Gists.Get(ctx, gistID)
		if err != nil {
			if resp != nil {
				switch resp.StatusCode {
				case http.StatusNotFound:
					http.Error(res, "gist not found", http.StatusNotFound)
					return
				case http.StatusForbidden:
					http.Error(res, "GitHub API rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			log.Println("failed to fetch gist:", err)
			http.Error(res, "failed to fetch gist", http.StatusBadGateway)
			return
		}
		if !gist.GetPublic() {
			http.Error(res, "gist not found", http.StatusNotFound)
			return
		}

		dir, err := gistToMemoryDirectory(gist, goExecPath)
		if err != nil {
			log.Println("failed to convert gist:", err)
			http.Error(res, "failed to load gist", http.StatusInternalServerError)
			return
		}

		data := Index{
			CopyrightNotice: fmt.Sprintf(CopyrightNotice, time.Now().Year()),
			GoVersion:       goVersion,
			Examples:        slices.Clone(examples),
			Name:            gistName(gist),
			Dir:             dir,
		}
		renderHTML(res, req, http.StatusOK, func(w io.Writer) error {
			return templates.ExecuteTemplate(w, "index.html.template", data)
		})
	}
}

func gistToMemoryDirectory(gist *github.Gist, goExecPath string) (MemoryDirectory, error) {
	files := gistFilesSorted(gist)

	// Case 1: Single .txt or .txtar file — parse as txtar
	if len(files) == 1 {
		ext := strings.ToLower(path.Ext(files[0].GetFilename()))
		if ext == ".txt" || ext == ".txtar" {
			archive := txtar.Parse([]byte(files[0].GetContent()))
			var filtered []txtar.File
			for _, f := range archive.Files {
				if !isPermittedFile(f.Name) {
					return MemoryDirectory{}, fmt.Errorf("file %s is not permitted", f.Name)
				}
				filtered = append(filtered, f)
			}
			archive.Files = filtered
			dir := MemoryDirectory{Archive: archive, MultiFile: true}
			expandNestedTxtar(&dir)
			return dir, nil
		}
	}

	// Case 2: Single .go file with package main — wrap with go.mod and run mod tidy
	if len(files) == 1 {
		f := files[0]
		if strings.ToLower(path.Ext(f.GetFilename())) == ".go" && isPackageMain([]byte(f.GetContent())) {
			return singleGoFileDirectory(f.GetFilename(), []byte(f.GetContent()), goExecPath)
		}
	}

	// Case 3: Multi-file gist — collect all permitted files
	archive := &txtar.Archive{}
	for _, f := range files {
		name := f.GetFilename()
		if !isPermittedFile(name) {
			return MemoryDirectory{}, fmt.Errorf("file %s is not permitted", name)
		}
		archive.Files = append(archive.Files, txtar.File{
			Name: name,
			Data: []byte(f.GetContent()),
		})
	}
	dir := MemoryDirectory{Archive: archive, MultiFile: true}
	expandNestedTxtar(&dir)
	return dir, nil
}

func singleGoFileDirectory(filename string, content []byte, goExecPath string) (MemoryDirectory, error) {
	goVersion := goVersionFromRuntime()

	archive := &txtar.Archive{
		Files: []txtar.File{
			{Name: "go.mod", Data: fmt.Appendf(nil, "module example.com\n\ngo %s\n", goVersion)},
			{Name: filename, Data: content},
		},
	}

	dir := MemoryDirectory{Archive: archive, MultiFile: true}
	fsDir := FilesystemDirectory{MemoryDirectory: dir}
	if err := fsDir.writeFiles(); err != nil {
		return dir, nil
	}
	defer func() { _ = fsDir.close() }()

	env := mergeEnv(os.Environ(), goEnvOverride()...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := fsDir.execGo(ctx, env, goExecPath, "mod", "tidy"); err != nil {
		// If mod tidy fails, return the directory as-is with the basic go.mod
		return dir, nil
	}

	if err := fsDir.readFiles(); err != nil {
		return dir, nil
	}

	return fsDir.MemoryDirectory, nil
}

func goVersionFromRuntime() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := readGoVersion(ctx)
	if err != nil {
		return "1.25"
	}
	return string(v)
}

func isPackageMain(src []byte) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.PackageClauseOnly)
	if err != nil {
		return false
	}
	return f.Name.Name == "main"
}

func gistName(gist *github.Gist) string {
	desc := strings.TrimSpace(gist.GetDescription())
	if desc == "" {
		return "Gist"
	}
	if len(desc) > 50 {
		return desc[:50]
	}
	return desc
}

func gistFilesSorted(gist *github.Gist) []github.GistFile {
	files := make([]github.GistFile, 0, len(gist.Files))
	for _, f := range gist.Files {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].GetFilename() < files[j].GetFilename()
	})
	return files
}
