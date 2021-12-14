package examples_test

import (
	"apply_codemod/src/apply"
	"apply_codemod/src/codemod"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func installPkg(pkgName string) func(codemod.Project) {
	return func(code codemod.Project) {
		fmt.Printf("cd %s\n", code.ProjectRoot)

		if err := os.Chdir(code.ProjectRoot); err != nil {
			fmt.Printf("unable to cd into %s", code.ProjectRoot)
			fmt.Println(err)
			return
		}

		fmt.Printf("installing %s\n", pkgName)
		err := exec.Command("go", "get", pkgName).Run()
		if err != nil {
			fmt.Printf("error installing package: %+v\n", err)
			return
		}

		fmt.Printf("installed %s\n", pkgName)
	}
}

func replaceImports(from, to string) func(*codemod.SourceFile) {
	return func(code *codemod.SourceFile) {
		for _, path := range code.Imports().Paths() {
			if strings.HasSuffix(path, from) {
				fmt.Printf("replacing import %s with %s\n", path, to)

				code.Imports().Remove(path)
				code.Imports().Add(to)

				return
			}
		}
	}
}

func deletePkgFolder(target string) func(codemod.Project) {
	return func(code codemod.Project) {
		filepath.Walk(code.ProjectRoot, func(path string, _ fs.FileInfo, _ error) error {
			if strings.HasSuffix(path, target) {
				os.RemoveAll(path)
			}

			return nil
		})
	}
}

func InstallPackageExample() {
	codemods :=
		[]apply.Codemod{
			{
				Description: "installs github.com/IQ-tech/go-errors",
				Transform:   installPkg("github.com/IQ-tech/go-errors"),
			},
			{
				Description: "imports github.com/IQ-tech/go-errors instead of {module}/infra/errors",
				Transform:   replaceImports("infra/errors", "github.com/IQ-tech/go-errors"),
			},
			{
				Description: "delete {module}infra/errors folder",
				Transform:   deletePkgFolder("infra/errors"),
			},
		}

	if err := apply.Locally(codemods); err != nil {
		panic(err)
	}
}
