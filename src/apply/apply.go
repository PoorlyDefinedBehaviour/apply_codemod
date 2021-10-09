package apply

import (
	"apply_codemod/src/apply/github"
	"apply_codemod/src/codemod"

	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"fmt"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const TEMP_FOLDER = "./codemod_tmp"

type Codemod struct {
	Description string
	Transform   func(*codemod.SourceFile)
}

type Repository struct {
	AccessToken string
	URL         string
	Branch      string
}

type Target struct {
	Repo     Repository
	Codemods []Codemod
}

func applyCodemodsToRepositoryFiles(codemods []Codemod) error {
	err := filepath.Walk(TEMP_FOLDER, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		file, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			return errors.WithStack(err)
		}

		sourceCode, err := ioutil.ReadAll(file)

		for _, mod := range codemods {

			if err != nil {
				return errors.WithStack(err)
			}

			code := codemod.New(sourceCode)

			mod.Transform(code)

			sourceCode = code.SourceCode()
			if err != nil {
				return errors.WithStack(err)
			}
		}

		err = file.Truncate(0)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = file.Seek(0, 0)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = file.Write(sourceCode)
		if err != nil {
			return errors.WithStack(err)
		}

		err = file.Sync()
		if err != nil {
			return errors.WithStack(err)
		}

		err = file.Close()
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})

	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func Codemods(targets []Target) error {
	for _, target := range targets {
		githubClient := github.New(github.Config{
			AccessToken: target.Repo.AccessToken,
		})

		err := os.RemoveAll(TEMP_FOLDER)
		if err != nil {
			return errors.WithStack(err)
		}

		repo, err := githubClient.Clone(github.CloneOptions{
			RepoURL: target.Repo.URL,
			Folder:  TEMP_FOLDER,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Checkout(github.CheckoutOptions{
			Branch: target.Repo.Branch,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		codemodBranch := uuid.New().String()

		err = repo.Checkout(github.CheckoutOptions{
			Branch: codemodBranch,
			Create: true,
			Force:  true,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		err = applyCodemodsToRepositoryFiles(target.Codemods)
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Add(github.AddOptions{
			All: true,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		FilesAffected, err := repo.FilesAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		if len(FilesAffected) == 0 {
			fmt.Printf("%s %s\n", color.RedString("[NOT CHANGED]"), target.Repo.URL)
			return nil
		}

		err = repo.Commit(
			"applied codemods",
			github.CommitOptions{All: true},
		)
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Push()
		if err != nil {
			return errors.WithStack(err)
		}

		pullRequest, err := githubClient.PullRequest(github.PullRequestOptions{
			RepoURL:     target.Repo.URL,
			Title:       "[AUTO GENERATED] applied codemods",
			FromBranch:  codemodBranch,
			ToBranch:    target.Repo.Branch,
			Description: buildDescription(&target),
		})
		if err != nil {
			return errors.WithStack(err)
		}

		fmt.Printf("%s %s\n", color.GreenString("[CREATED]"), *pullRequest.HTMLURL)
	}

	return nil
}

func buildDescription(target *Target) string {
	builder := strings.Builder{}

	builder.WriteString("Applied the following codemods:\n\n")

	for i, codemod := range target.Codemods {
		builder.WriteString(fmt.Sprintf("λ %s", codemod.Description))

		if i < len(target.Codemods)-1 {
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}
