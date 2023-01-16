package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const CopyrightNotice = "© 2021-%d Christopher Hunter"

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
