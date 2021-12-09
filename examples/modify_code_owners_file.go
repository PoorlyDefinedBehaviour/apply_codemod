package examples_test

import (
	"apply_codemod/src/codemod"
	"fmt"
	"os"
	"github.com/pkg/errors"
)

// Creates or updates .github/CODEOWNERS
func modifyRepository(code codemod.Project) {
	err := os.MkdirAll(fmt.Sprintf("%s/.github", code.ProjectRoot), os.ModePerm)
	if err != nil {
		fmt.Printf("couldn't create .github folder => %+v", errors.WithStack(err))
		return
	}

	file, err := os.OpenFile(
		fmt.Sprintf("%s/.github/CODEOWNERS", code.ProjectRoot),
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
