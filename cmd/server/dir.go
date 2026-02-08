package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/tools/txtar"
)

type FilesystemDirectory struct {
	MemoryDirectory
	TempDir string
	Modules []Module

	Output bytes.Buffer
}

func newFilesystemDirectory(md MemoryDirectory) (FilesystemDirectory, error) {
	dir := FilesystemDirectory{MemoryDirectory: md}
	if err := dir.checkDependencies(); err != nil {
		return dir, err
	}
	if err := dir.writeFiles(); err != nil {
		return dir, fmt.Errorf("failed to write files %s: %w", dir.close(), err)
	}
	return dir, nil
}

func newRequestDirectory(req *http.Request) (FilesystemDirectory, error) {
	dir, err := readMemoryDirectory(req)
	if err != nil {
		return FilesystemDirectory{}, err
	}
	return newFilesystemDirectory(dir)
}

func (dir *FilesystemDirectory) checkDependencies() error {
	mods, err := checkDependencies(dir.Archive)
	if err != nil {
		return err
	}
	dir.Modules = append(dir.Modules, mods...)
	return err
}

func (dir *FilesystemDirectory) close() error {
	if dir.TempDir == "" {
		return nil
	}
	if err := os.RemoveAll(dir.TempDir); err != nil {
		slog.Error("failed to remove temporary directory", "dir", dir.TempDir)
		return fmt.Errorf("failed to delete temporary directory")
	}
	return nil
}

func (dir *FilesystemDirectory) readFiles() error {
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

func (dir *FilesystemDirectory) writeFiles() error {
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

func (dir *FilesystemDirectory) execGo(ctx context.Context, env []string, goExecPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, goExecPath, args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &dir.Output)
	cmd.Stderr = io.MultiWriter(os.Stdout, &dir.Output)
	cmd.Env = env
	cmd.Dir = dir.TempDir
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
