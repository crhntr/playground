package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"testing"

	"golang.org/x/tools/txtar"
)

func Test_permittedPackages(t *testing.T) {
	perm := permittedPackages()
	if slices.Contains(perm, "") {
		t.Error("empty string not allowed")
	}
}

// TestExamples ensures every embedded example is accepted by the dependency
// allow lists and, outside of short mode, builds the way the run handler
// builds it (this needs the go command and network access for modules).
func TestExamples(t *testing.T) {
	names, err := fs.Glob(assets, "assets/examples/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no examples found")
	}
	for _, name := range names {
		t.Run(path.Base(name), func(t *testing.T) {
			buf, err := fs.ReadFile(assets, name)
			if err != nil {
				t.Fatal(err)
			}
			dir, err := newFilesystemDirectory(MemoryDirectory{Archive: txtar.Parse(buf)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = dir.close() })

			if testing.Short() {
				t.Skip("skipping build in short mode")
			}
			goExecPath, err := exec.LookPath("go")
			if err != nil {
				t.Skip("go command not available")
			}
			env := mergeEnv(os.Environ(), goEnvOverride()...)
			if _, err := dir.buildWASM(t.Context(), env, goExecPath); err != nil {
				t.Fatal(err)
			}
		})
	}
}
