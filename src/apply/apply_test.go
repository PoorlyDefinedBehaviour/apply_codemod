package apply

import (
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
