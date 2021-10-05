package codemod_test

import (
	"apply_codemod/src/codemod"
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Map(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
	package main

	func main() {
		x := map[string]string{
			"transaction_isolation": "'READ-COMMITED'",
		}
	}
`)

	t.Run("Has", func(t *testing.T) {
		_, literal := codemod.New(sourceCode).FindMapLiteral("map[string]string")

		t.Run("returns true if map contains key", func(t *testing.T) {
			assert.True(t, literal.Has("transaction_isolation"))
		})

		t.Run("returns false if map does not contain key", func(t *testing.T) {
			assert.False(t, literal.Has("key_not_in_the_map"))
		})
	})

	t.Run("RenameKey", func(t *testing.T) {
		_, literal := codemod.New(sourceCode).FindMapLiteral("map[string]string")

		t.Run("renames the key", func(t *testing.T) {
			expected := `map[string]string{"tx_isolation": "'READ-COMMITED'"}`

			literal.RenameKey("transaction_isolation", "tx_isolation")

			actual := codemod.SourceCode(literal.Expr.Node)

			assert.Equal(t, expected, actual)
		})

		t.Run("if key is not in the map, does nothing", func(t *testing.T) {
			_, literal := codemod.New(sourceCode).FindMapLiteral("map[string]string")

			expected := `map[string]string{"transaction_isolation": "'READ-COMMITED'"}`

			literal.RenameKey("a", "b")

			actual := codemod.SourceCode(literal.Expr.Node)

			assert.Equal(t, expected, actual)
		})
	})
}

func Test_FindIfStatements(t* testing.T){
	t.Parallel()


	// sourceCode := []byte(`
	// package main

	// func main() {
	// 	if true {
	// 		println(2)
	// 	}
	// }
	// `)

	t.Run("foo",func(t *testing.T){
		// for _, statements := range codemod.New(sourceCode).FindIfStatements() {
		// 	for _, statement := range statements {
		// 		// if statement.Condition().SourceCode() == "true" {
		// 		// 	statement.Remove()
		// 		// }
		// 		{

		// 		}
		// 	}
		// }
	})
}

func Test_RewriteErrorsWrapfToFmtErrorf(t*testing.T){
	t.Parallel()

	sourceCode := []byte(`
	package main

	import "errors"

	var errSomething = errors.New("oops")

	func foo() error {
		return errors.Wrapf(errSomething, "some context")
	}

	func main() {
		
	}
	`)

	file := codemod.New(sourceCode)

	scopedCalls := file.FindCalls("errors.Wrapf")

	for _, calls := range scopedCalls {
		for _, call := range calls {
			originalArgs := call.Node.Args

			// swap first and last arguments
			newArgs := originalArgs[1:]

			newArgs = append(newArgs, originalArgs[0])

			// add the error format to the string
			newArgs[0] = &ast.BasicLit{
				Value: 
				codemod.Quote(
					codemod.Unquote(
						newArgs[0].(*ast.BasicLit).Value) + ": %w",
				),
			}

			// ast node representing fmt.Errorf(...)
			newCall := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.Ident{Name:"fmt"},
					Sel: &ast.Ident{Name:"Errorf"},
				},
				Args: newArgs,
			}			
			
			// rewrite the ast and call fmt.Errorf instead of errors.Wrap
			call.Replace(newCall)
		}
	}

	if len(scopedCalls) > 0 {
		imports := file.Imports()

		if !imports.Some(func(path string) bool { return path == "fmt" }){
			imports.Add("fmt")
		}
	}

	expected := 
`package main

import (
	"errors"
	"fmt"
)

var errSomething = errors.New("oops")

func foo() error {
	return fmt.Errorf("some context: %w", errSomething)
}

func main() {

}
`

	updatedSourceCode := string(file.SourceCode())

	assert.Equal(t, expected, updatedSourceCode)
}