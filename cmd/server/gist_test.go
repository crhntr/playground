package main

import (
	"testing"

	"github.com/google/go-github/v80/github"
	"golang.org/x/tools/txtar"
)

func Test_isPackageMain(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "package main", src: "package main\n", want: true},
		{name: "package foo", src: "package foo\n", want: false},
		{name: "empty", src: "", want: false},
		{name: "invalid", src: "not go code", want: false},
		{name: "main with imports", src: "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPackageMain([]byte(tt.src)); got != tt.want {
				t.Errorf("isPackageMain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gistName(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{name: "empty description", desc: "", want: "Gist"},
		{name: "whitespace only", desc: "   ", want: "Gist"},
		{name: "short description", desc: "My gist", want: "My gist"},
		{name: "exactly 50 chars", desc: "01234567890123456789012345678901234567890123456789", want: "01234567890123456789012345678901234567890123456789"},
		{name: "long description", desc: "This is a very long description that exceeds the fifty character limit for gist names", want: "This is a very long description that exceeds the f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gist := &github.Gist{Description: &tt.desc}
			if got := gistName(gist); got != tt.want {
				t.Errorf("gistName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_gistToMemoryDirectory_txtar(t *testing.T) {
	content := `-- go.mod --
module example.com

go 1.25
-- main.go --
package main

func main() {}
`
	gist := &github.Gist{
		Files: map[github.GistFilename]github.GistFile{
			"playground.txtar": {
				Filename: github.Ptr("playground.txtar"),
				Content:  &content,
			},
		},
	}

	dir, err := gistToMemoryDirectory(gist, "go")
	if err != nil {
		t.Fatal(err)
	}

	if len(dir.Archive.Files) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(dir.Archive.Files))
	}

	hasGoMod := false
	hasMainGo := false
	for _, f := range dir.Archive.Files {
		switch f.Name {
		case "go.mod":
			hasGoMod = true
		case "main.go":
			hasMainGo = true
		}
	}
	if !hasGoMod {
		t.Error("expected go.mod file")
	}
	if !hasMainGo {
		t.Error("expected main.go file")
	}
}

func Test_gistToMemoryDirectory_multiFile(t *testing.T) {
	goMod := "module example.com\n\ngo 1.25\n"
	mainGo := "package main\n\nfunc main() {}\n"
	secret := "SECRET=value\n"

	gist := &github.Gist{
		Files: map[github.GistFilename]github.GistFile{
			"go.mod": {
				Filename: github.Ptr("go.mod"),
				Content:  &goMod,
			},
			"main.go": {
				Filename: github.Ptr("main.go"),
				Content:  &mainGo,
			},
			".env": {
				Filename: github.Ptr(".env"),
				Content:  &secret,
			},
		},
	}

	dir, err := gistToMemoryDirectory(gist, "go")
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range dir.Archive.Files {
		if f.Name == ".env" {
			t.Error(".env should be filtered out by isPermittedFile")
		}
	}

	hasGoMod := false
	hasMainGo := false
	for _, f := range dir.Archive.Files {
		switch f.Name {
		case "go.mod":
			hasGoMod = true
		case "main.go":
			hasMainGo = true
		}
	}
	if !hasGoMod {
		t.Error("expected go.mod file")
	}
	if !hasMainGo {
		t.Error("expected main.go file")
	}
}

func Test_gistFilesSorted(t *testing.T) {
	gist := &github.Gist{
		Files: map[github.GistFilename]github.GistFile{
			"z.go": {Filename: github.Ptr("z.go")},
			"a.go": {Filename: github.Ptr("a.go")},
			"m.go": {Filename: github.Ptr("m.go")},
		},
	}
	sorted := gistFilesSorted(gist)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 files, got %d", len(sorted))
	}
	if sorted[0].GetFilename() != "a.go" || sorted[1].GetFilename() != "m.go" || sorted[2].GetFilename() != "z.go" {
		t.Errorf("files not sorted: %v", sorted)
	}
}

func Test_gistToMemoryDirectory_txtFile(t *testing.T) {
	content := string(txtar.Format(&txtar.Archive{
		Files: []txtar.File{
			{Name: "go.mod", Data: []byte("module example.com\n\ngo 1.25\n")},
			{Name: "main.go", Data: []byte("package main\n\nfunc main() {}\n")},
		},
	}))

	gist := &github.Gist{
		Files: map[github.GistFilename]github.GistFile{
			"playground.txt": {
				Filename: github.Ptr("playground.txt"),
				Content:  &content,
			},
		},
	}

	dir, err := gistToMemoryDirectory(gist, "go")
	if err != nil {
		t.Fatal(err)
	}

	if len(dir.Archive.Files) < 2 {
		t.Fatalf("expected at least 2 files, got %d", len(dir.Archive.Files))
	}
}
