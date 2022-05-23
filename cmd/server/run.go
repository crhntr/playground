package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

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
