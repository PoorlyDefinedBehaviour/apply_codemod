package apply

import (
	"apply_codemod/src/codemod"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

func isGoFile(filename string) bool {
	return strings.HasSuffix(filename, ".go")
}

func compileRegexes(regexes map[string]string) (map[*regexp.Regexp]string, error) {
	out := make(map[*regexp.Regexp]string, len(regexes))

	for regexToCompile, replacement := range regexes {
		re, err := regexp.Compile(regexToCompile)
		if err != nil {
			return out, errors.WithStack(err)
		}

		out[re] = replacement
	}

	return out, nil
}

// Traverses `directory` and applies each codemod to each Go file in
// `directory` and its subdirectories.
//
// Replacements are applied to every file.
//
// The vendor folder is ignored.
func applyCodemodsToDirectory(directory string, replacements map[string]string, codemods []sourceFileCodemod) (err error) {
	// If we have nothing to do with the repository files,
	// we won't wast time traversing the directory.
	if len(replacements) == 0 && len(codemods) == 0 {
		return nil
	}

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

	replacementRegexes, err := compileRegexes(replacements)
	if err != nil {
		return errors.WithStack(err)
	}

	err = filepath.Walk("./", func(path string, info fs.FileInfo, _ error) error {
		if strings.Contains(path, "vendor") || info == nil || info.IsDir() {
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

		for re, replacement := range replacementRegexes {
			sourceCode = re.ReplaceAll(sourceCode, []byte(replacement))
		}

		if isGoFile(info.Name()) {
			code, err := codemod.New(codemod.NewInput{
				SourceCode: sourceCode,
				FilePath:   path,
			})
			if err != nil {
				return errors.WithStack(err)
			}

			for _, mod := range codemods {
				if err != nil {
					return errors.WithStack(err)
				}

				mod.transform(code)

				sourceCode = code.SourceCode()
				if err != nil {
					return errors.WithStack(err)
				}
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
