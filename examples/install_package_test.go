package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/apply"
	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/codemod"
)

func installPkg(pkgName string) func(codemod.Project) {
	return func(code codemod.Project) {
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
		projectRoot, _ := os.Getwd()

		filepath.Walk(projectRoot, func(path string, _ fs.FileInfo, _ error) error {
			if strings.HasSuffix(path, target) {
				os.RemoveAll(path)
			}

			return nil
		})
	}
}

// Example usage
// go run main.go \
// -token=github_token \
// -repo=https://github.com/PoorlyDefinedBehaviour/apply_codemod_test \
// -branch=main \
// -pkg=github.com/IQ-tech/go-errors \
// -path=infra/errors
func main() {
	token := flag.String("token", "", "github access token")
	pkg := flag.String("pkg", "", "package that will be installed")
	path := flag.String("path", "", "path of the package that's being replaced")
	repo := flag.String("repo", "", "target repo")
	branch := flag.String("branch", "", "target branch")

	flag.Parse()

	if *token == "" {
		flag.Usage()
		panic("github token is required")
	}

	if *pkg == "" {
		flag.Usage()
		panic("the package that will be installed is required")
	}

	if *path == "" {
		flag.Usage()
		panic("the package that's being replaced by the new package is required. example: infra/errors")
	}

	if *repo == "" {
		flag.Usage()
		panic("the target repo is required")
	}

	if *branch == "" {
		flag.Usage()
		panic("the repo branch is required")
	}

	if err := apply.Apply(context.Background(), []apply.Codemod{
		{
			Description: fmt.Sprintf("installs %s", *pkg),
			Transform:   installPkg(*pkg),
		},
		{
			Description: fmt.Sprintf("imports %s instead of %s", *pkg, *path),
			Transform:   replaceImports(*path, *pkg),
		},
		{
			Description: fmt.Sprintf("delete %s", *path),
			Transform:   deletePkgFolder(*path),
		},
	}); err != nil {
		panic(err)
	}
}
