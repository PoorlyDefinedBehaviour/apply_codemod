package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRepoURL(t *testing.T) {
	t.Parallel()

	t.Run("parses valid url", func(t *testing.T) {
		t.Parallel()
		expected := RepoInfo{
			Owner: "PoorlyDefinedBehaviour",
			Name:  "apply_codemod_test",
		}

		actual := parseRepoURL("https://github.com/PoorlyDefinedBehaviour/apply_codemod_test")

		assert.Equal(t, expected, actual)
	})
}
