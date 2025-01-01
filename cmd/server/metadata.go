package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const CopyrightNotice = "© 2021-%d Christopher Hunter"

func readGoVersion(ctx context.Context) ([]byte, error) {
	goExecPath, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}

	env := mergeEnv(os.Environ(), goEnvOverride()...)

	re := regexp.MustCompile(`go(?P<version>\d+[.\-\w]*)`)
	versionMatchIndex := re.SubexpIndex("version")

	cmd := exec.CommandContext(ctx, goExecPath, "version")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return nil, errors.New(buf.String())
	}
	output, err := io.ReadAll(&buf)
	if err != nil {
		return nil, errors.New("failed to read command output")
	}
	matches := re.FindSubmatch(output)
	if len(matches) < versionMatchIndex {
		return nil, errors.New("failed to read version from output")
	}

	return matches[versionMatchIndex], nil
}

func handleVersion(goVersion string) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("content-type", "text/plain")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(goVersion))
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
