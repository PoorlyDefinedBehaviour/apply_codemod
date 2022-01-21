package main_test

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/codemod"
	"github.com/stretchr/testify/assert"
)

// Moves context.Context to the first position in
// interface, function declarations and function arguments.
func Test_IfContextIsTheLastArgumentItBecomesTheFirst(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
	package main

	import "context"

	type UserService interface {
		DoSomething(int64, context.Context) error
	}

	func buz(userID int64, ctx context.Context) error {
		return nil
	}
	
	func baz(userID int64, context context.Context) error {
		return buz(userID, context)
	}
	
	func foo(userID int64, ctx context.Context) error {
		err := baz(userID, ctx)
		if err != nil {
			return err
		}
		return nil
	}

	func main() {
		_ = foo(1, context.Background())
	}
	`)

	file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

	// find function declarations
	// example:
	// func foo(x int) {}
	for _, function := range file.Functions() {
		params := function.Params()

		// for each function parameter
		// example:
		// func(x int, y string) {}
		// we would go through x and then y
		for i, param := range params {
			// we are looking for the type Context from any package.
			// we will match these two for example:
			// context.Context
			// othercontext.Context
			if !strings.HasSuffix(codemod.SourceCode(param.Type), ".Context") {
				continue
			}

			// swap context with first position argument
			params[0], params[i] = params[i], params[0]
		}
	}

	for _, calls := range file.FunctionCalls() {
		for _, call := range calls {
			for i, arg := range call.Node.Args {
				if expr, ok := arg.(*ast.CallExpr); ok {
					// if we are calling context.Background()
					if fun, ok := expr.Fun.(*ast.SelectorExpr); ok {
						if fun.X.(*ast.Ident).Name == "context" && fun.Sel.Name == "Background" {
							// swap context argument with the first position argument
							call.Node.Args[0], call.Node.Args[i] = call.Node.Args[i], call.Node.Args[0]
						}
					}
				}

				if expr, ok := arg.(*ast.Ident); ok {
					// if we are passing context.Context as argument to a function
					// example:
					// foo(userID, ctx)
					if expr.Name == "ctx" || expr.Name == "context" {
						call.Node.Args[0], call.Node.Args[i] = call.Node.Args[i], call.Node.Args[0]
					}
				}
			}
		}
	}

	for _, typeDecl := range file.TypeDeclarations() {
		for _, method := range typeDecl.Methods() {
			params := method.Params()

			for i, param := range params {
				if codemod.SourceCode(param.Type) == "context.Context" {
					params[0], params[i] = params[i], params[0]
				}
			}
		}
	}

	expected :=
		`package main

import "context"

type UserService interface {
	DoSomething(context.Context, int64) error
}

func buz(ctx context.Context, userID int64) error {
	return nil
}

func baz(context context.Context, userID int64) error {
	return buz(context, userID)
}

func foo(ctx context.Context, userID int64) error {
	err := baz(ctx, userID)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	_ = foo(context.Background(), 1)
}
`

	updatedSourceCode := string(file.SourceCode())

	assert.Equal(t, expected, updatedSourceCode)
}
