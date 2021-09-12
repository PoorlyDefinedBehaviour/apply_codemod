## Example

```go

package main

import (
	"apply_codemod/src/apply"
	"apply_codemod/src/codemod"
	"flag"
	"go/ast"
)
/*
Goes from:

errors.Wrapf(...)

to:

fmt.Errorf(...)
*/
func rewriteErrorsWrapfToFmtErrorf(file *codemod.SourceFile) {
	scopedCalls := file.FindCalls("errors.Wrapf")

	for _, calls := range scopedCalls {
		for _, call := range calls {
			originalArgs := call.Args()

			// swap first and last arguments
			newArgs := originalArgs[1:]

			newArgs = append(newArgs, originalArgs[0])

			// add the error format to the string
			newArgs[0].SetString(newArgs[0].String() + ": %w")

			// ast node representing fmt.Errorf(...)
			newCall := &ast.ExprStmt{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.Ident{
							Name: "fmt",
						},
						Sel: &ast.Ident{
							Name: "Errorf",
						},
					},
					Args: newArgs.Unwrap(),
				},
			}

			// rewrite the ast and call fmt.Errorf instead of errors.Wrap
			call.Replace(newCall)
		}
	}

	if len(scopedCalls) > 0 {
		imports := file.Imports()

		if !imports.Contains("fmt") {
			imports.Add("fmt")
		}
	}
}

func main() {
	accessToken := flag.String("token", "", "github access token")

	flag.Parse()

	if accessToken == nil {
		panic("github access token is required")
	}

	codemods := []apply.Target{
		{

			Repo: apply.Repository{
				AccessToken: *accessToken,
				URL:         "https://github.com/PoorlyDefinedBehaviour/apply_codemod_test",
				Branch:      "main",
			},
			Codemods: []apply.Codemod{
				{
					Description: "replaces errors.Wrapf with fmt.Errorf",
					Transform:   rewriteErrorsWrapfToFmtErrorf,
				},
			},
		},
	}
	err := apply.Codemods(codemods)
	if err != nil {
		panic(err)
	}
}
```
