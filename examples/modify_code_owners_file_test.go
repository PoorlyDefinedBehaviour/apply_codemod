package main

import (
	"apply_codemod/src/codemod"
	"fmt"
	"os"

	"github.com/pkg/errors"
)

// Creates or updates .github/CODEOWNERS
func modifyRepository(code codemod.Project) {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Printf("couldn't find project root => %+v", err)
		return
	}

	err = os.MkdirAll(fmt.Sprintf("%s/.github", projectRoot), os.ModePerm)
	if err != nil {
		fmt.Printf("couldn't create .github folder => %+v", errors.WithStack(err))
		return
	}

	file, err := os.OpenFile(
		fmt.Sprintf("%s/.github/CODEOWNERS", projectRoot),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		os.ModePerm,
	)
	if err != nil {
		fmt.Printf("couldn't open nor create => %+v", errors.WithStack(err))
		return
	}

	defer file.Close()

	newFileContents := "* @poorlydefinedbehaviour"

	_, err = file.Write([]byte(newFileContents))
	if err != nil {
		fmt.Printf("couldn't modify CODEOWNERS file => %+v", errors.WithStack(err))
	}
}
