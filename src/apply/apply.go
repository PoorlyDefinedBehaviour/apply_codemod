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

		err = filepath.Walk(TEMP_FOLDER, func(path string, info fs.FileInfo, err error) error {
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}

			for _, mod := range target.Codemods {
				file, err := os.OpenFile(path, os.O_RDWR, 0644)
				if err != nil {
					return errors.WithStack(err)
				}

				originalSourceCode, err := ioutil.ReadAll(file)
				if err != nil {
					return errors.WithStack(err)
				}

				code := codemod.New(originalSourceCode)

				mod.Transform(code)

				updatedSourceCodeBytes := code.SourceCode()
				if err != nil {
					return errors.WithStack(err)
				}

				err = file.Truncate(0)
				if err != nil {
					return errors.WithStack(err)
				}

				_, err = file.Seek(0, 0)
				if err != nil {
					return errors.WithStack(err)
				}

				_, err = file.Write(updatedSourceCodeBytes)
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
			}

			return nil
		})
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Add(github.AddOptions{
			All: true,
		})
		if err != nil {
			return errors.WithStack(err)
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

		err = githubClient.PullRequest(github.PullRequestOptions{
			RepoURL:     target.Repo.URL,
			Title:       "[AUTO GENERATED] applied codemods",
			FromBranch:  codemodBranch,
			ToBranch:    target.Repo.Branch,
			Description: buildDescription(&target),
		})
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func buildDescription(target *Target) string {
	descriptions := make([]string, len(target.Codemods))

	for _, codemod := range target.Codemods {
		descriptions = append(descriptions, codemod.Description)
	}

	return fmt.Sprintf(
		"Applied the following codemods: %s",
		strings.Join(descriptions, "\n\nλ "),
	)
}
