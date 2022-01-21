package apply

import (
	"apply_codemod/src/codemod"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func Test_buildPullRequestDescription(t *testing.T) {
	t.Parallel()

	codemods := []sourceFileCodemod{
		{description: "a"},
		{description: "b"},
		{description: "c"},
	}

	expected :=
		`Applied the following codemods:

λ a
λ b
λ c
`

	os.Args = []string{
		"directory",
		"--github_token=token",
		"--repo_name_matches=apply_codemod_test",
		"--local_dir=repository/dir/in/my/computer",
	}

	applier, err := New()

	applier.sourceFileCodemods = codemods

	assert.NoError(t, err)

	actual := applier.buildPullRequestDescription()

	assert.Equal(t, expected, actual)
}

func Test_applyCodemodsToDirectory(t *testing.T) {
	t.Parallel()

	testFolder := fmt.Sprintf("%s/Test_applyCodemodsToDirectory", tempFolder)

	assert.Nil(t, os.MkdirAll(testFolder, os.ModePerm))

	assert.Nil(t, ioutil.WriteFile(
		fmt.Sprintf("%s/file.go", testFolder),
		[]byte(`
		package main 

		func main() {}
	`),
		os.ModePerm,
	))

	t.Run("on panic", func(t *testing.T) {
		t.Run("if reason is an error, returns it", func(t *testing.T) {
			panicErr := errors.New("oops")

			mods := []sourceFileCodemod{
				{
					description: "will panic",
					transform:   func(_ *codemod.SourceFile) { panic(panicErr) },
				},
			}

			err := applyCodemodsToDirectory(tempFolder, map[string]string{}, mods)

			assert.Equal(t, panicErr, err)
		})

		t.Run("if reason is not an error, creates an error with the reason and returns it", func(t *testing.T) {
			mods := []sourceFileCodemod{
				{
					description: "will panic",
					transform:   func(_ *codemod.SourceFile) { panic("a") },
				},
			}

			err := applyCodemodsToDirectory(tempFolder, map[string]string{}, mods)

			assert.Equal(t, "unexpected panic => a", err.Error())
		})
	})

	t.Run("on success", func(t *testing.T) {
		t.Run("returns nil", func(t *testing.T) {
			mods := []sourceFileCodemod{
				{
					description: "no-op",
					transform:   func(_ *codemod.SourceFile) {},
				},
			}

			assert.Nil(t, applyCodemodsToDirectory(tempFolder, map[string]string{}, mods))
		})
	})
}

func TestLocally(t *testing.T) {
	t.Parallel()

	// t.Run("when directory where codemods should be applied is not informed", func(t *testing.T) {
	// 	t.Run("returns error", func(t *testing.T) {
	// 		err := Locally([]Codemod{})

	// 		assert.True(t, errors.Is(err, ErrDirIsRequired))
	// 	})
	// })

	// TODO:
	// Test that codemods are actually applied to temp folder.
	// Create a folder for text features.
}

func Test_New(t *testing.T) {
	t.Parallel()

	t.Run("the profile or a list of repositories must be informed", func(t *testing.T) {
		os.Args = []string{"directory", "--github_token=token"}

		_, err := New()

		assert.True(t, errors.Is(err, ErrArgumentIsRequired))
		assert.Equal(
			t,
			"If a list of repositories is not informed, a github user or organization must be: argument is required",
			err.Error(),
		)
	})

	t.Run("parses arguments", func(t *testing.T) {
		os.Args = []string{
			"directory",
			"--github_token=token",
			"--repo_name_matches=apply_codemod_test",
			"--local_dir=repository/dir/in/my/computer",
		}

		applier, err := New()

		assert.NoError(t, err)

		repoNameMatches := "apply_codemod_test"
		localDir := "repository/dir/in/my/computer"

		expected := CliArgs{
			GithubToken:     "token",
			RepoNameMatches: &repoNameMatches,
			LocalDirectory:  &localDir,
			Repositories:    map[string]string{},
			Replacements:    map[string]string{},
		}

		assert.Equal(t, expected, applier.args)
	})
}
