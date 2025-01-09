package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/txtar"
)

type Directory struct {
	TempDir string
	Modules []Module
	Archive *txtar.Archive
}

func newFilesystemDirectory(archive *txtar.Archive) (Directory, error) {
	var dir Directory

	if err := dir.checkDependencies(archive); err != nil {
		return dir, err
	}
	dir.Archive = archive

	if err := dir.writeFiles(); err != nil {
		return dir, fmt.Errorf("failed to write files %s: %w", dir.close(), err)
	}

	return dir, nil
}

func newRequestDirectory(req *http.Request) (Directory, error) {
	archive, err := newRequestArchive(req)
	if err != nil {
		return Directory{}, err
	}
	return newFilesystemDirectory(archive)
}

func (dir *Directory) checkDependencies(archive *txtar.Archive) error {
	mods, err := checkDependencies(archive)
	if err != nil {
		return err
	}
	dir.Modules = append(dir.Modules, mods...)
	return err
}

func (dir *Directory) close() error {
	if dir.TempDir == "" {
		return nil
	}
	if err := os.RemoveAll(dir.TempDir); err != nil {
		slog.Error("failed to remove temporary directory", "dir", dir.TempDir)
		return fmt.Errorf("failed to delete temporary directory")
	}
	return nil
}

func (dir *Directory) readFiles() error {
	for i, file := range dir.Archive.Files {
		p := filepath.Join(dir.TempDir, filepath.FromSlash(file.Name))
		buf, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("failed to read: %s", file.Name)
		}
		dir.Archive.Files[i].Data = buf
	}
	return nil
}

func (dir *Directory) writeFiles() error {
	tmp, err := os.MkdirTemp("", "")
	if err != nil {
		log.Println("failed to create temporary directory", err)
		return fmt.Errorf("failed to create temporary directory")
	}
	dir.TempDir = tmp
	dirFS, err := txtar.FS(dir.Archive)
	if err != nil {
		return errors.Join(err, dir.close())
	}
	if err := os.CopyFS(tmp, dirFS); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return nil
}

func (dir *Directory) execGo(ctx context.Context, env []string, out io.Writer, goExecPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, goExecPath, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, out)
	cmd.Stderr = io.MultiWriter(os.Stdout, out)
	cmd.Env = env
	cmd.Dir = dir.TempDir
	io.WriteString(out, "$ "+strings.Join(append([]string{path.Base(cmd.Path)}, cmd.Args...), " "))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
