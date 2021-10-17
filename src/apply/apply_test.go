package apply

import (
	"fmt"
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

func Test_isVendorFolder(t *testing.T) {
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
		assert.Equal(t, tt.expected, isVendorFolder(tt.path))
	}
}
