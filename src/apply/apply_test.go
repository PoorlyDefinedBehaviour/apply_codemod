package apply

import (
	"apply_codemod/src/codemod"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_buildDescription(t *testing.T) {
	t.Parallel()

	target := Target{
		Repo: Repository{
			AccessToken: "access_token",
			URL:         "https://github.com/PoorlyDefinedBehaviour/apply_codemod_test",
			Branch:      "main",
		},
		Codemods: []Codemod{
			{Description: "a"},
			{Description: "b"},
			{Description: "c"},
		},
	}

	expected :=
		`Applied the following codemods:

λ a

λ b

λ c`

	actual := buildDescription(&target)

	assert.Equal(t, expected, actual)
}

func Test_isFileInsideVendorFolder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{
			path:     fmt.Sprintf("%s/src/main.go", tempFolder),
			expected: false,
		},
		{
			path:     fmt.Sprintf("%s/vendor/package/a.go", tempFolder),
			expected: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, isFileInsideVendorFolder(tt.path))
	}
}

func Test_applyCodemodsToRepositoryFiles(t *testing.T) {
	t.Parallel()

	testFolder := fmt.Sprintf("%s/Test_applyCodemodsToRepositoryFiles", tempFolder)

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

			mods := []Codemod{
				{
					Description: "will panic",
					Transform:   func(_ *codemod.SourceFile) { panic(panicErr) },
				},
			}

			err := applyCodemodsToRepositoryFiles(mods)

			assert.Equal(t, panicErr, err)
		})

		t.Run("if reason is not an error, creates an error with the reason and returns it", func(t *testing.T) {
			mods := []Codemod{
				{
					Description: "will panic",
					Transform:   func(_ *codemod.SourceFile) { panic("a") },
				},
			}

			err := applyCodemodsToRepositoryFiles(mods)

			assert.Equal(t, "unexpected panic => a", err.Error())
		})
	})

	t.Run("on success", func(t *testing.T) {
		t.Run("returns nil", func(t *testing.T) {
			mods := []Codemod{
				{
					Description: "no-op",
					Transform:   func(_ *codemod.SourceFile) {},
				},
			}

			assert.Nil(t, applyCodemodsToRepositoryFiles(mods))
		})
	})
}
