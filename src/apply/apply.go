package apply

import (
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"apply_codemod/src/apply/github"
	"apply_codemod/src/codemod"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const tempFolder = "./codemod_tmp"

type Codemod struct {
	Description string
	Transform   interface{}
}

type Repository struct {
	URL    string
	Branch string
}

type Target struct {
	AccessToken  string
	Repositories []Repository
	Codemods     []Codemod
}

func applyCodemodsToDirectory(directory string, codemods []Codemod) (err error) {
	defer func() {
		if reason := recover(); reason != nil {
			panicErr, ok := reason.(error)
			if !ok {
				err = errors.Errorf("unexpected panic => %+v", reason)
			} else {
				err = panicErr
			}
		}
	}()

	err = filepath.Walk("./", func(path string, info fs.FileInfo, _ error) error {
		if strings.Contains(path, "vendor") || info == nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		file, err := os.OpenFile(path, os.O_RDWR, 0o644)
		if err != nil {
			return errors.WithStack(err)
		}

		sourceCode, err := ioutil.ReadAll(file)
		if err != nil {
			return errors.WithStack(err)
		}

		code, err := codemod.New(codemod.NewInput{
			SourceCode:  sourceCode,
			FilePath:    path,
			ProjectRoot: directory,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		for _, mod := range codemods {
			if err != nil {
				return errors.WithStack(err)
			}

			if f, ok := mod.Transform.(func(*codemod.SourceFile)); ok {
				f(code)
			}

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

var ErrDirIsRequired = errors.New("directory where codemods should be applied is required")

func Locally(mods []Codemod) error {
	targetDirectoryPath := flag.String("dir", "", "directory where codemods should be applied")

	flag.Parse()

	if *targetDirectoryPath == "" {
		flag.Usage()
		return errors.WithStack(ErrDirIsRequired)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := os.Chdir(*targetDirectoryPath); err != nil {
		return errors.WithStack(err)
	}

	for _, mod := range mods {
		if f, ok := mod.Transform.(func(codemod.Project)); ok {
			f(codemod.Project{ProjectRoot: *targetDirectoryPath})
		}
	}

	if err := applyCodemodsToDirectory(*targetDirectoryPath, mods); err != nil {
		return errors.WithStack(err)
	}

	if err := os.Chdir(originalDir); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func Codemods(target Target) error {
	for _, repository := range target.Repositories {
		githubClient := github.New(github.Config{
			AccessToken: target.AccessToken,
		})

		if err := os.RemoveAll(tempFolder); err != nil {
			return errors.WithStack(err)
		}

		repo, err := githubClient.Clone(github.CloneOptions{
			RepoURL: repository.URL,
			Folder:  tempFolder,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		err = repo.Checkout(github.CheckoutOptions{
			Branch: repository.Branch,
		})
		if err != nil {
			return errors.Wrapf(err, "git checkout %s failed in %s", repository.Branch, repository.URL)
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

		originalDir, err := os.Getwd()
		if err != nil {
			return errors.WithStack(err)
		}

		if err := os.Chdir(tempFolder); err != nil {
			return errors.WithStack(err)
		}

		for _, mod := range target.Codemods {
			if f, ok := mod.Transform.(func(codemod.Project)); ok {
				f(codemod.Project{ProjectRoot: tempFolder})
			}
		}

		err = applyCodemodsToDirectory(tempFolder, target.Codemods)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := os.Chdir(originalDir); err != nil {
			return errors.WithStack(err)
		}

		err = repo.Add(github.AddOptions{
			All: true,
		})
		if err != nil {
			return errors.WithStack(err)
		}

		filesAffected, err := repo.FilesAffected()
		if err != nil {
			return errors.WithStack(err)
		}
		if len(filesAffected) == 0 {
			fmt.Printf("%s %s\n", color.RedString("[NOT CHANGED]"), repository.URL)
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
			RepoURL:     repository.URL,
			Title:       "[AUTO GENERATED] applied codemods",
			FromBranch:  codemodBranch,
			ToBranch:    repository.Branch,
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
