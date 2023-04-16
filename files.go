package playground

import (
	_ "embed"
	"io/fs"
)

func ListFileNames(dir fs.ReadDirFS, p string) []string {
	var result []string
	entries, _ := dir.ReadDir(p)
	for _, e := range entries {
		if !e.Type().IsDir() {
			continue
		}
		result = append(result, e.Name())
	}
	return result
}
