package codemod_test

import (
	"apply_codemod/src/codemod"
	"go/ast"
	"strings"
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

func Test_FindIfStatements(t *testing.T) {
	t.Parallel()

	// sourceCode := []byte(`
	// package main

	// func main() {
	// 	if true {
	// 		println(2)
	// 	}
	// }
	// `)

	t.Run("foo", func(t *testing.T) {
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

func Test_RewriteErrorsWrapfToFmtErrorf(t *testing.T) {
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
			args := call.Node.Args

			args[0], args[len(args)-1] = args[len(args)-1], args[0]

			args[0].(*ast.BasicLit).Value = codemod.Quote(codemod.Unquote(args[0].(*ast.BasicLit).Value) + ": %w")

			call.Node.Fun = &ast.SelectorExpr{
				X:   &ast.Ident{Name: "fmt"},
				Sel: &ast.Ident{Name: "Errorf"},
			}
		}
	}

	if len(scopedCalls) > 0 {
		imports := file.Imports()

		if !imports.Some(func(path string) bool { return path == "fmt" }) {
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

	file := codemod.New(sourceCode)

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

func TestSourceFile_FunctionCalls(t *testing.T) {
	t.Parallel()

	t.Run("when there are no function calls", func(t *testing.T) {
		t.Run("returns the empty map", func(t *testing.T) {
			file := codemod.New([]byte(`
				package main 

				func a() {}

				func main() {}
			`))

			assert.Empty(t, file.FunctionCalls())
		})
	})

	t.Run("when there are function calls", func(t *testing.T) {
		t.Run("returns them", func(t *testing.T) {
			t.Run("identifier call", func(t *testing.T) {
				file := codemod.New([]byte(`
				package main 
	
				func a() {}
	
				func main() {
					a()
				}
			`))

				scopes := file.FunctionCalls()

				assert.Equal(t, 1, len(scopes))

				for _, calls := range scopes {
					for _, call := range calls {
						assert.Equal(t, "a", call.Node.Fun.(*ast.Ident).Name)
					}
				}
			})

			t.Run("selector call", func(t *testing.T) {
				file := codemod.New([]byte(`
				package main 

				import "errors"
	
				func main() {
					_ = errors.New("oops")
				}
			`))

				scopes := file.FunctionCalls()

				assert.Equal(t, 1, len(scopes))

				for _, calls := range scopes {
					for _, call := range calls {
						selector := call.Node.Fun.(*ast.SelectorExpr)

						assert.Equal(t, "errors", selector.X.(*ast.Ident).Name)
						assert.Equal(t, "New", selector.Sel.Name)
					}
				}
			})
		})
	})
}

func TestSourceFile_Functions(t *testing.T) {
	t.Parallel()

	t.Run("when there are function declarations", func(t *testing.T) {
		t.Run("returns them", func(t *testing.T) {
			file := codemod.New([]byte(`
			package main 

			func inc(x int) int {
				return x + 1
			}

			func main() {}
		`))

			functions := file.Functions()

			assert.Equal(t, 2, len(functions))

			assert.Equal(t, "inc", functions[0].Node.Name.Name)
			assert.Equal(t, "main", functions[1].Node.Name.Name)
		})
	})

	t.Run("when there are no function declarations", func(t *testing.T) {
		t.Run("returns nothing", func(t *testing.T) {
			file := codemod.New([]byte(`
				package foo

				var SomeConstant int64 = 1
			`))

			assert.Empty(t, file.Functions())
		})
	})
}

func Test_TypeDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("when there are no type declarations", func(t *testing.T) {
		t.Run("returns nothing", func(t *testing.T) {
			file := codemod.New([]byte(`
			package main

			func main(){}
		`))

			assert.Empty(t, file.TypeDeclarations())
		})
	})

	t.Run("when there are type declarations", func(t *testing.T) {
		t.Run("returns interfaces", func(t *testing.T) {
			file := codemod.New([]byte(`
			package main

			type UserService interface {}

			func main(){}
		`))

			declarations := file.TypeDeclarations()

			assert.Equal(t, 1, len(declarations))
			assert.Equal(t, "UserService", declarations[0].Node.Name.Name)
		})

		t.Run("returns structs", func(t *testing.T) {
			file := codemod.New([]byte(`
			package main

			type UserService struct {}

			func main(){}
		`))

			declarations := file.TypeDeclarations()

			assert.Equal(t, 1, len(declarations))
			assert.Equal(t, "UserService", declarations[0].Node.Name.Name)
		})

		t.Run("returns type aliases", func(t *testing.T) {
			file := codemod.New([]byte(`
			package main

			type UserID int64

			func main(){}
		`))

			declarations := file.TypeDeclarations()

			assert.Equal(t, 1, len(declarations))
			assert.Equal(t, "UserID", declarations[0].Node.Name.Name)
		})
	})
}

func TestTypeDeclaration_Methods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected []string
	}{
		{
			code: `
					package foo

					type Interface interface {}

					type Struct struct {}

					type Foo int
					`,
			expected: []string{},
		},
		{
			code: `
				package main

				import "context"

				type Interface interface {
					Foo(int64, context.Context) error
					Bar()
					Baz(int) string
				}

				func main(){}
			`,
			expected: []string{"Foo", "Bar", "Baz"},
		},
		{
			code: `
				package foo

				type User struct {}

				func (user *User) IsAdmin() bool { return false }
				func (user User) Something() {}
			`,
			expected: []string{"IsAdmin", "Something"},
		},
		{
			code: `
				package foo

				type ID int64

				func (id *ID) A() string { return "hello" }
				func (id ID) B() error { return nil }
			`,
			expected: []string{"A", "B"},
		},
	}

	for _, tt := range tests {
		declarations := codemod.New([]byte(tt.code)).TypeDeclarations()

		assert.NotEmpty(t, declarations)

		for _, declaration := range declarations {
			for i := range tt.expected {
				assert.Equal(t, tt.expected[i], declaration.Methods()[i].Name())
			}
		}
	}
}

func TestTypeDeclaration_IsInterface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected bool
	}{
		{
			code: `
				package main 

				type User struct{}
			`,
			expected: false,
		},
		{
			code: `
				package main 

				type UserID int64
			`,
			expected: false,
		},
		{
			code: `
				package main 

				type I interface {}
			`,
			expected: true,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, codemod.New([]byte(tt.code)).TypeDeclarations()[0].IsInterface())
	}
}

func TestTypeDeclaration_IsStruct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected bool
	}{
		{
			code: `
				package main 

				type User struct{}
			`,
			expected: true,
		},
		{
			code: `
				package main 

				type UserID int64
			`,
			expected: false,
		},
		{
			code: `
				package main 

				type I interface {}
			`,
			expected: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, codemod.New([]byte(tt.code)).TypeDeclarations()[0].IsStruct())
	}
}

func TestTypeDeclaration_IsTypeAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected bool
	}{
		{
			code: `
				package main

				type User struct{}
			`,
			expected: false,
		},
		{
			code: `
				package main 

				type UserID int64
			`,
			expected: true,
		},
		{
			code: `
				package main

				type I interface {}
			`,
			expected: false,
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, codemod.New([]byte(tt.code)).TypeDeclarations()[0].IsTypeAlias())
	}
}

func TestMethod_Name(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected string
	}{
		{
			code: `
				package main

				type I interface {
					Foo(int64) int64
				}
				`,
			expected: "Foo",
		},
		{
			code: `
				package main

				type S struct {}

				func (s *S) Bar() {}
				`,
			expected: "Bar",
		},
		{
			code: `
				package main

				type S struct {}

				func (s S) Bar() {}
				`,
			expected: "Bar",
		},
		{
			code: `
			package main 

			type T int64

			func (t T) Baz() {}
			`,
			expected: "Baz",
		},
		{
			code: `
			package main 

			type T int64

			func (t *T) Baz() {}
			`,
			expected: "Baz",
		},
	}

	for _, tt := range tests {
		declarations := codemod.New([]byte(tt.code)).TypeDeclarations()

		actual := declarations[0].Methods()[0].Name()

		assert.Equal(t, tt.expected, actual)
	}
}
