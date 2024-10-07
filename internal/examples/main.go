package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

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
		dirFS, err := txtar.FS(archive)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.CopyFS(tmpDir, dirFS); err != nil {
			log.Fatal(err)
		}

		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = tmpDir
		fmt.Println(tidy.Args)
		tidy.Stderr = os.Stdout
		tidy.Stdout = os.Stdout
		if err := tidy.Run(); err != nil {
			log.Fatal(err)
		}

		get := exec.Command("go", "get", "-u", "./...")
		get.Dir = tmpDir
		fmt.Println(get.Args)
		get.Stderr = os.Stdout
		get.Stdout = os.Stdout
		if err := get.Run(); err != nil {
			log.Fatal(err)
		}

		format := exec.Command("gofumpt", "-w", ".")
		format.Dir = tmpDir
		fmt.Println(format.Args)
		format.Stderr = os.Stdout
		format.Stdout = os.Stdout
		if err := format.Run(); err != nil {
			log.Fatal(err)
		}

		for i, file := range archive.Files {
			updated, err := os.ReadFile(filepath.Join(tmpDir, filepath.FromSlash(file.Name)))
			if err != nil {
				log.Fatal(err)
			}
			archive.Files[i].Data = updated
		}

		if err := os.WriteFile(match, txtar.Format(archive), matchInfo.Mode().Perm()); err != nil {
			log.Fatal(err)
		}
	}
}
