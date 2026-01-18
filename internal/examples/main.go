package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/crhntr/txtarfmt"
	"golang.org/x/tools/txtar"
)

func main() {
	examplesDirPath := filepath.FromSlash("cmd/server/assets/examples")
	matches, err := filepath.Glob(filepath.Join(examplesDirPath, "*.txtar"))
	if err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "")
	if err != nil {
		log.Fatal(err)
	}
	goModVersion := "1.23"
	bi, ok := debug.ReadBuildInfo()
	if ok && bi.GoVersion != "" {
		goModVersion = strings.TrimLeft(bi.GoVersion, "vgo")
	}
	for _, match := range matches {
		fmt.Println(match)
		buf, err := os.ReadFile(match)
		if err != nil {
			log.Fatal(err)
		}
		matchInfo, err := os.Stat(match)
		if err != nil {
			log.Fatal(err)
		}
		archive := txtar.Parse(buf)

		tmpDir := filepath.Join(tmp, strings.TrimSuffix(path.Base(match), ".txtar"))
		if err := os.MkdirAll(tmpDir, 0744); err != nil {
			log.Fatal(err)
		}
		commands := []*exec.Cmd{
			exec.Command("go", "mod", "tidy"),
			exec.Command("go", "mod", "edit", "-go", goModVersion),
			exec.Command("go", "get", "-u", fmt.Sprintf(".%c...", filepath.Separator)),
			exec.Command("gofumpt", "-w", "."),
			exec.Command("go", "build", "-v", "."),
		}
		for i := range commands {
			commands[i].Env = append([]string{"GOOS=js", "GOARCH=wasm"}, commands[i].Environ()...)
			commands[i].Stderr = os.Stdout
			commands[i].Stdout = os.Stdout
		}

		updated, err := txtarfmt.Execute(tmpDir, []string{"playground"}, archive, commands...)
		if err != nil {
			log.Fatal(err)
		}
		archive.Files = updated.Files

		if err := os.WriteFile(match, txtar.Format(archive), matchInfo.Mode().Perm()); err != nil {
			log.Fatal(err)
		}
	}
}
