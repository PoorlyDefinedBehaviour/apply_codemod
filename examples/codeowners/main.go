package main

import (
	"context"
	"fmt"
	"os"

	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/apply"
	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/codemod"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
)

// Creates or updates .github/CODEOWNERS
func modifyRepository(codeowners string) func(codemod.Project) {
	return func(code codemod.Project) {
		fmt.Printf("creating or updating codemods file. codeowners=%s\n", codeowners)

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

		fmt.Printf("creating codeowners file, project_root=%s\n", projectRoot)
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

		_, err = file.Write([]byte(codeowners))
		if err != nil {
			fmt.Printf("couldn't modify CODEOWNERS file => %+v", errors.WithStack(err))
		}
	}
}

type args struct {
	Codeowners string `long:"codeowners" description:"the CODEOWNERS file contents"`
}

/*
USAGE:

go run main.go \
--github_token=token \
--repos=https://github.com/PoorlyDefinedBehaviour/repo_1,https://github.com/PoorlyDefinedBehaviour/repo_2 \
--codeowners="* @poorlydefinedbehaviour @user2 @user3"
*/
func main() {
	var args args

	// We use the default parser options and
	// also ignore unknown arguments.
	_, err := flags.NewParser(&args, flags.Default|flags.IgnoreUnknown|flags.HelpFlag).ParseArgs(os.Args[1:])
	if err != nil {
		panic(err)
	}

	err = apply.Apply(context.Background(), []apply.Codemod{
		{
			Description: "create or update CODEOWNERS file",
			Transform:   modifyRepository(args.Codeowners),
		},
	})
	if err != nil {
		panic(err)
	}
}
