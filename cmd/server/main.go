package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	domAST "github.com/crhntr/window/ast"

	"github.com/crhntr/playground/view"
)

const CopyrightNotice = "© %d Christopher Hunter"

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

		defer func() {
			_ = req.Body.Close()
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

func createMainFile(dir string, rc io.Reader) error {
	fp := filepath.Join(dir, "main.go")
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = io.Copy(f, io.LimitReader(rc, 1<<15))
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

func readGoVersion(ctx context.Context) []byte {
	goExecPath, lookUpErr := exec.LookPath("go")
	if lookUpErr != nil {
		panic(lookUpErr)
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	re := regexp.MustCompile(`go(?P<version>\d+[.\-\w]*)`)
	versionMatchIndex := re.SubexpIndex("version")

	cmd := exec.CommandContext(ctx, goExecPath, "version")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Env = env
	err := cmd.Run()
	if err != nil {
		panic(buf.String())
	}
	output, err := io.ReadAll(&buf)
	if err != nil {
		panic("failed to read command output")
	}
	matches := re.FindSubmatch(output)
	if len(matches) < versionMatchIndex {
		panic("failed to read version from output")
	}

	return matches[versionMatchIndex]
}

func handleVersion() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Second*2)
		defer cancel()

		version := readGoVersion(ctx)

		res.Header().Set("content-type", "text/plain")
		res.WriteHeader(http.StatusOK)
		res.Write(version)
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
