package main

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", http.FileServer(http.FS(assetsFS))))
	mux.Handle("/", http.HandlerFunc(handlePage))

	mux.Handle("/go/version", handleVersion())
	mux.Handle("/go/run", handleRun())

	err := http.ListenAndServe(":8080", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		log.Println(req.URL)
		mux.ServeHTTP(res, req)
	}))
	if err != nil {
		panic(err)
	}
}

var (
	//go:embed webapp
	webappFS embed.FS
	assetsFS fs.FS

	pages = map[string]string{
		"/":    "webapp/index.gohtml",
		"/run": "webapp/run.gohtml",
	}
)

func init() {
	var err error
	assetsFS, err = fs.Sub(webappFS, "webapp/assets")
	if err != nil {
		panic(err)
	}
}

func handleRun() http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second*5)
		defer cancel()

		tmp, err := os.MkdirTemp("", "")
		if err != nil {
			http.Error(res, "failed to create temporary directory", http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = os.RemoveAll(tmp)
		}()

		err = createMainFile(tmp, req.Body)
		if err != nil {
			http.Error(res, "failed to write main.go", http.StatusInternalServerError)
			return
		}
		err = createModFile(tmp)
		if err != nil {
			http.Error(res, "failed to create go.mod", http.StatusInternalServerError)
			return
		}

		const output = "main.wasm"
		cmd := exec.CommandContext(ctx, goExecPath,
			"build",
			"-o", output,
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Env = env
		cmd.Dir = tmp
		err = cmd.Run()
		if err != nil {
			http.Error(res, stderr.String(), http.StatusInternalServerError)
			return
		}

		b, err := os.Open(filepath.Join(tmp, output))
		if err != nil {
			http.Error(res, "failed to open build file", http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = b.Close()
		}()
		mw := multipart.NewWriter(res)
		defer func() {
			_ = mw.Close()
		}()
		res.Header().Set("Content-Type", mw.FormDataContentType())
		res.WriteHeader(http.StatusOK)

		err = mw.WriteField("stdout", stdout.String())
		if err != nil {
			log.Println(err)
			return
		}
		err = mw.WriteField("stderr", stderr.String())
		if err != nil {
			log.Println(err)
			return
		}
		wasm, err := mw.CreateFormFile("output", "main.wasm")
		if err != nil {
			log.Println(err)
			return
		}
		_, _ = io.Copy(wasm, b)
	}
}

func createMainFile(dir string, rc io.ReadCloser) error {
	fp := filepath.Join(dir, "main.go")
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = io.Copy(f, rc)
	if err != nil {
		return err
	}
	return nil
}

func createModFile(dir string) error {
	fp := filepath.Join(dir, "go.mod")
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = f.Write([]byte("module playground\n"))
	if err != nil {
		return err
	}
	return nil
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

func handleVersion() http.HandlerFunc {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second*2)
		defer cancel()

		cmd := exec.CommandContext(ctx, goExecPath, "version")
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmd.Env = env
		err := cmd.Run()
		if err != nil {
			http.Error(res, buf.String(), http.StatusInternalServerError)
			return
		}
		res.WriteHeader(http.StatusOK)
		_, _ = io.Copy(res, &buf)
	}
}

func goEnvOverride() []string { return []string{"GOOS=js", "GOARCH=wasm"} }

func mergeEnv(env []string, additional ...string) []string {
	l := len(env) + len(env)
	m := make(map[string]string, l)
	keys := make([]string, 0, l)
	for _, v := range append(additional, env...) {
		x := strings.Index(v, "=")
		k := v[:x]
		keys = append(keys, k)
		m[k] = v[x+1:]
	}
	result := make([]string, 0, l)
	for _, k := range keys {
		result = append(result, k+"="+m[k])
	}
	return result
}
