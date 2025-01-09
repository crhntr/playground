package main

import (
	_ "embed"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/txtar"
)

func newRequestArchive(req *http.Request) (*txtar.Archive, error) {
	const maxReadBytes = (1 << 10) * 8
	_ = req.ParseMultipartForm(maxReadBytes)
	defer closeAndIgnoreError(req.Body)
	return readArchive(req.Form)
}

func readArchive(form url.Values) (*txtar.Archive, error) {
	filenames := form["filename"]
	archive := &txtar.Archive{Files: make([]txtar.File, 0, len(filenames))}
	for _, filename := range filenames {
		archive.Files = append(archive.Files, txtar.File{
			Name: filename,
			Data: []byte(form.Get(filename)),
		})
	}
	return archive, nil
}

//go:embed assets/module_allow_list.txt
var permittedModulesString string

type Module struct {
	File   txtar.File
	Module modfile.File
}

func newModule(file txtar.File) (Module, error) {
	module, err := modfile.Parse(file.Name, file.Data, nil)
	if err != nil {
		return Module{}, err
	}
	if len(module.Replace) != 0 {
		return Module{}, fmt.Errorf("replace directive is not allowed in module")
	}
	allowed := strings.Split(permittedModulesString, "\n")
	allowed = slices.DeleteFunc(allowed, func(s string) bool {
		return s == ""
	})
	for _, requirement := range module.Require {
		if requirement.Indirect {
			continue
		}
		if !slices.Contains(allowed, requirement.Mod.Path) {
			return Module{}, fmt.Errorf("module %s not permitted", requirement.Mod.Path)
		}
	}
	return Module{
		File:   file,
		Module: *module,
	}, nil
}

func checkImports(fileName string, src []byte) error {
	var fileSet token.FileSet
	file, err := parser.ParseFile(&fileSet, fileName, src, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("failed to parse main.go: %w", err)
	}
	allowedModules := strings.Split(permittedModulesString, "\n")
	allowedModules = slices.DeleteFunc(allowedModules, func(s string) bool {
		return s == ""
	})

	for _, spec := range file.Imports {
		pkgPath, _ := strconv.Unquote(spec.Path.Value)
		if slices.Index(permittedPackages(), pkgPath) >= 0 {
			continue
		}
		if slices.ContainsFunc(allowedModules, func(modName string) bool {
			return strings.HasPrefix(pkgPath, modName+"/")
		}) {
			continue
		}
		return fmt.Errorf("package %q not permitted", pkgPath)
	}
	return nil
}

//go:embed assets/import_allow_list.txt
var permittedPackagesString string

func permittedPackages() []string {
	return removeZeros(strings.Split(permittedPackagesString, "\n"))
}

func checkDependencies(archive *txtar.Archive) ([]Module, error) {
	var modules []Module
	for _, file := range archive.Files {
		base := path.Base(file.Name)
		switch {
		case base == "go.mod":
			mod, err := newModule(file)
			if err != nil {
				return nil, errors.Join(err, err)
			}
			modules = append(modules, mod)
		case path.Ext(base) == ".go":
			if err := checkImports(file.Name, file.Data); err != nil {
				return nil, fmt.Errorf("failed in %s: %w", file.Name, err)
			}
		}
	}
	return modules, nil
}
