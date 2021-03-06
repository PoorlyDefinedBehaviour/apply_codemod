package codemod_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/codemod"
	"github.com/stretchr/testify/assert"
)

func check(t *testing.T, a, b string) {
	t.Helper()

	assert.Equal(t, codemod.NormalizeString(a), codemod.NormalizeString(b))
}

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
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

		_, literal := file.FindMapLiteral("map[string]string")

		t.Run("returns true if map contains key", func(t *testing.T) {
			assert.True(t, literal.Has("transaction_isolation"))
		})

		t.Run("returns false if map does not contain key", func(t *testing.T) {
			assert.False(t, literal.Has("key_not_in_the_map"))
		})
	})

	t.Run("RenameKey", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

		_, literal := file.FindMapLiteral("map[string]string")

		t.Run("renames the key", func(t *testing.T) {
			expected := `map[string]string{"tx_isolation": "'READ-COMMITED'"}`

			literal.RenameKey("transaction_isolation", "tx_isolation")

			actual := codemod.SourceCode(literal.Expr.Node)

			check(t, expected, actual)
		})

		t.Run("if key is not in the map, does nothing", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

			_, literal := file.FindMapLiteral("map[string]string")

			expected := `map[string]string{"transaction_isolation": "'READ-COMMITED'"}`

			literal.RenameKey("a", "b")

			actual := codemod.SourceCode(literal.Expr.Node)

			check(t, expected, actual)
		})
	})
}

func Test_IfStatements(t *testing.T) {
	t.Parallel()

	t.Run("finds if statement", func(t *testing.T) {
		t.Parallel()

		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			if true { }
		}
	`)})

		scopedStatements := file.IfStatements()

		assert.Equal(t, 1, len(scopedStatements))

		for _, statements := range scopedStatements {
			assert.Equal(t, 1, len(statements))

			for _, statement := range statements {
				check(t, "true", codemod.SourceCode(statement.Node.Cond))
			}
		}
	})

	t.Run("removal", func(t *testing.T) {
		sourceCode := []byte(`
		package main

		func main() {
			if true {
				println(2)
			}
		}
	`)

		t.Run("removes if statement", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

			for _, statements := range file.IfStatements() {
				for _, statement := range statements {
					statement.Remove()
				}
			}

			expected := "package main\n\nfunc main() {\n\n}\n"

			actual := string(file.SourceCode())

			check(t, expected, actual)
		})

		t.Run("removes only if statement condition", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

			for _, statements := range file.IfStatements() {
				for _, statement := range statements {
					statement.RemoveCondition()
				}
			}

			expected := "package main\n\nfunc main() {\n\n\tprintln(2)\n\n}\n"

			actual := string(file.SourceCode())

			check(t, expected, actual)
		})
	})
}

func Test_ReplaceDatabaseConnectionErrorIfStatement(t *testing.T) {
	t.Parallel()

	t.Skip()

	sourceCode := []byte(`
	package database

	import (
		"database/sql"
		"time"
	
		"github.com/go-sql-driver/mysql"
		"github.com/pkg/errors"
	)
	
	func Connect() (*sql.DB, error) {
		config := mysql.Config{
			User:   "mysql",
			Passwd: "mysql",
			DBName: "db",
			Params: map[string]string{
				"tx_isolation": "'READ-COMMITTED'",
			},
		}
	
		db, err := sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return db, errors.WithStack(err)
		}
	
		err = db.Ping()
		if err != nil {
			if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1193 {
				config.Params["tx_isolation"], config.Params["transaction_isolation"] = config.Params["transaction_isolation"], config.Params["tx_isolation"]
	
				db, err = sql.Open("mysql", config.FormatDSN())
				if err != nil {
					return db, errors.WithStack(err)
				}
			}
		}

		if err != nil {
			return db, errors.WithStack(err)
		}
	
		db.SetConnMaxLifetime(time.Minute * 3)
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(10)
	
		return db, nil
	}	
	`)

	file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

	for _, statements := range file.IfStatements() {
		for _, statement := range statements {
			if !strings.HasSuffix(codemod.SourceCode(statement.Node.Cond), "Number == 1193") {
				continue
			}

			body := codemod.Ast(`
			config.Params["tx_isolation"], config.Params["transaction_isolation"] = config.Params["transaction_isolation"], config.Params["tx_isolation"]
				if config.Params["tx_isolation"] == "" {
					delete(config.Params, "tx_isolation")
				}

				if config.Params["transaction_isolation"] == "" {
					delete(config.Params, "transaction_isolation")
				}

				db, err = sql.Open("mysql", config.FormatDSN())
				if err != nil {
					return db, errors.WithStack(err)
				}

				err = db.Ping()
				if err != nil {
					return db, errors.WithStack(err)
				}
			`)

			statement.Node.Body = body.(*ast.BlockStmt)
		}
	}

	expected :=
		`package database

import (
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

func Connect() (*sql.DB, error) {
	config := mysql.Config{
		User:   "mysql",
		Passwd: "mysql",
		DBName: "db",
		Params: map[string]string{
			"tx_isolation": "'READ-COMMITTED'",
		},
	}

	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return db, errors.WithStack(err)
	}

	err = db.Ping()
	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1193 {
			config.Params["tx_isolation"], config.Params["transaction_isolation"] = config.Params["transaction_isolation"], config.Params["tx_isolation"]
			if config.Params["tx_isolation"] == "" {
				delete(config.Params, "tx_isolation")
			}
			if config.Params["transaction_isolation"] == "" {
				delete(config.Params, "transaction_isolation")
			}
			db, err = sql.Open("mysql", config.FormatDSN())
			if err != nil {
				return db, errors.WithStack(err)
			}
			err = db.Ping()
			if err != nil {
				return db, errors.WithStack(err)
			}
		}

	}

	if err != nil {
		return db, errors.WithStack(err)
	}

	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db, nil
}
`

	actual := string(file.SourceCode())

	check(t, expected, actual)
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

	file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

	scopedCalls := file.FunctionCalls()

	for _, calls := range scopedCalls {
		for _, call := range calls {
			if call.FunctionName() != "errors.Wrapf" {
				continue
			}

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
		file.Imports().Add("fmt")
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

func TestSourceFile_FunctionCalls(t *testing.T) {
	t.Parallel()

	t.Run("when there are no function calls", func(t *testing.T) {
		t.Run("returns the empty map", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
				package main 

				func a() {}

				func main() {}
			`)})

			assert.Empty(t, file.FunctionCalls())
		})
	})

	t.Run("when there are function calls", func(t *testing.T) {
		t.Run("returns them", func(t *testing.T) {
			t.Run("identifier call", func(t *testing.T) {
				file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
				package main 
	
				func a() {}
	
				func main() {
					a()
				}
			`)})

				scopes := file.FunctionCalls()

				assert.Equal(t, 1, len(scopes))

				for _, calls := range scopes {
					for _, call := range calls {
						assert.Equal(t, "a", call.Node.Fun.(*ast.Ident).Name)
					}
				}
			})

			t.Run("selector call", func(t *testing.T) {
				file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
				package main 

				import "errors"
	
				func main() {
					_ = errors.New("oops")
				}
			`)})

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

func Test_FunctionCall(t *testing.T) {
	t.Parallel()

	t.Run("returns function name", func(t *testing.T) {
		tests := []struct {
			code     string
			expected string
		}{
			{
				code: `
			package helloworld
	
			func f() {}
	
			func g() {
				f()
			}
		`,
				expected: "f",
			},
			{
				code: `
			package main
	
			import "errors"
	
			func main() {
				_ = errors.New("oops")
			}
		`,
				expected: "errors.New",
			},
		}

		for _, tt := range tests {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

			scopedCalls := file.FunctionCalls()

			assert.Equal(t, 1, len(scopedCalls))

			for _, calls := range scopedCalls {
				assert.Equal(t, 1, len(calls))

				assert.Equal(t, tt.expected, calls[0].FunctionName())
			}
		}
	})

	t.Run("inserts node before function call", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			import "somepackage"

			func main() {
				somepackage.Foo()
			}
		`)})

		for _, calls := range file.FunctionCalls() {
			for _, call := range calls {
				if call.FunctionName() == "somepackage.Foo" {
					call.InsertBefore(codemod.Ast(`x := 1`))
				}
			}
		}

		expected :=
			`package main

import "somepackage"

func main() {
	x := 1
	somepackage.Foo()
}
`

		check(t, expected, string(file.SourceCode()))
	})

	t.Run("inserts node after function call", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			import "somepackage"

			func main() {
				somepackage.Foo()
			}
		`)})

		for _, calls := range file.FunctionCalls() {
			for _, call := range calls {
				if call.FunctionName() == "somepackage.Foo" {
					node := codemod.Ast(`
						type S struct {}
					`)

					call.InsertAfter(node)
				}
			}
		}

		expected :=
			`package main

import "somepackage"

func main() {
	somepackage.Foo()
	type S struct {}
}
`

		actual := string(file.SourceCode())

		check(t, expected, actual)
	})

	t.Run("removes function call", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			import "somepackage"

			func main() {
				somepackage.Foo()
			}
		`)})

		for _, calls := range file.FunctionCalls() {
			for _, call := range calls {
				if call.FunctionName() == "somepackage.Foo" {
					call.Remove()
				}
			}
		}

		expected :=
			`package main

import "somepackage"

func main() {

}
`

		actual := string(file.SourceCode())

		check(t, expected, actual)
	})
}

func TestSourceFile_Functions(t *testing.T) {
	t.Parallel()

	t.Run("when there are function declarations", func(t *testing.T) {
		t.Run("returns them", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main 

			func inc(x int) int {
				return x + 1
			}

			func main() {}
		`)})

			functions := file.Functions()

			assert.Equal(t, 2, len(functions))

			assert.Equal(t, "inc", functions[0].Node.Name.Name)
			assert.Equal(t, "main", functions[1].Node.Name.Name)
		})
	})

	t.Run("when there are no function declarations", func(t *testing.T) {
		t.Run("returns nothing", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
				package foo

				var SomeConstant int64 = 1
			`)})

			assert.Empty(t, file.Functions())
		})
	})
}

func Test_TypeDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("when there are no type declarations", func(t *testing.T) {
		t.Run("returns nothing", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			func main(){}
		`)})

			assert.Empty(t, file.TypeDeclarations())
		})
	})

	t.Run("when there are type declarations", func(t *testing.T) {
		t.Run("returns interfaces", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			type UserService interface {}

			func main(){}
		`)})

			declarations := file.TypeDeclarations()

			assert.Equal(t, 1, len(declarations))
			assert.Equal(t, "UserService", declarations[0].Node.Name.Name)
		})

		t.Run("returns structs", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			type UserService struct {}

			func main(){}
		`)})

			declarations := file.TypeDeclarations()

			assert.Equal(t, 1, len(declarations))
			assert.Equal(t, "UserService", declarations[0].Node.Name.Name)
		})

		t.Run("returns type aliases", func(t *testing.T) {
			file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			type UserID int64

			func main(){}
		`)})

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
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		declarations := file.TypeDeclarations()

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
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		assert.Equal(t, tt.expected, file.TypeDeclarations()[0].IsInterface())
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
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		assert.Equal(t, tt.expected, file.TypeDeclarations()[0].IsStruct())
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
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		assert.Equal(t, tt.expected, file.TypeDeclarations()[0].IsTypeAlias())
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
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		declarations := file.TypeDeclarations()

		actual := declarations[0].Methods()[0].Name()

		assert.Equal(t, tt.expected, actual)
	}
}

func TestSourceFile_FindAssignments(t *testing.T) {
	t.Parallel()

	file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
			package main

			func main() {
				a := 1

				b := make(map[string]string)

				b["tx_isolation"] = "'READ-COMMITTED'"

				c := struct{
					x int
				}{
					x: 0,
				}

				c.x = 1

				d := make([]int, 1)

				d[0] = 10

				d = append(d, 20)
			}
		`)})

	tests := []struct {
		target   string
		expected string
	}{
		{target: "a", expected: "a := 1"},
		{target: "a := 1", expected: "a := 1"},
		{target: "a:=1", expected: "a := 1"},
		{target: "b := make(map[string]string)", expected: "b := make(map[string]string)"},
		{target: `b["tx_isolation"]`, expected: `b["tx_isolation"] = "'READ-COMMITTED'"`},
		{target: "c.x", expected: "c.x = 1"},
		{target: "c.x = 1", expected: "c.x = 1"},
		{target: "d[0]", expected: "d[0] = 10"},
		{target: "d[0] = 10", expected: "d[0] = 10"},
		{target: "d = append(d, 20)", expected: "d = append(d, 20)"},
	}

	for _, tt := range tests {
		scopedAssignments := file.FindAssignments(tt.target)

		assert.Equal(t, 1, len(scopedAssignments))

		for _, assignments := range scopedAssignments {
			assert.Equal(t, 1, len(assignments))

			assert.Equal(t, tt.expected, codemod.SourceCode(assignments[0].Node))
		}
	}
}

func Test_Assignments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected []string
	}{
		{
			code: `
			package main

			func main() {}
			`,
			expected: []string{},
		},
		{
			code: `
			package main

			func main() {
				x := 1

				x = 2
			}
			`,
			expected: []string{"x := 1", "x = 2"},
		},
	}

	for _, tt := range tests {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(tt.code)})

		actual := file.Assignments()

		for _, assignments := range actual {
			assert.Equal(t, len(tt.expected), len(assignments))

			for i := range tt.expected {
				assert.Equal(t, tt.expected[i], codemod.SourceCode(assignments[i].Node))
			}
		}
	}
}

func TestAst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected ast.Node
	}{
		{
			code:     "x := 1",
			expected: &ast.AssignStmt{},
		},
		{
			code: `
			if config.Params["tx_isolation"] == "" {
				delete(config.Params, "tx_isolation")
			}
			`,
			expected: &ast.IfStmt{},
		},
		{
			code:     "type I interface {}",
			expected: &ast.DeclStmt{},
		},
		{
			code:     "type S struct {}",
			expected: &ast.DeclStmt{},
		},
		{
			code: `
				if a > 2 {

				}

				if b > 5 {

				}
			`,
			expected: &ast.BlockStmt{},
		},
	}

	for _, tt := range tests {
		actual := codemod.Ast(tt.code)

		assert.Equal(t, fmt.Sprintf("%T", tt.expected), fmt.Sprintf("%T", actual))
	}
}

func TestAssign_InsertAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected string
	}{
		{
			code: "y := 2",
			expected: `package main

func main() {
	x := 1
	y := 2

}
`,
		},
		{
			code: `
				if config.Params["tx_isolation"] == "" {
					delete(config.Params, "tx_isolation")
				}
			`,
			expected: `package main

func main() {
	x := 1
	if config.Params["tx_isolation"] == "" {
		delete(config.Params, "tx_isolation")
	}

}
`,
		},
	}

	for _, tt := range tests {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			x := 1
		}
	`)})

		for _, assignments := range file.FindAssignments("x") {
			for _, assignment := range assignments {
				assignment.InsertAfter(codemod.Ast(tt.code))
			}
		}

		actual := string(file.SourceCode())

		check(t, tt.expected, actual)
	}
}

func TestAssign_InsertBefore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     string
		expected string
	}{
		{
			code: "y := 2",
			expected: `package main

func main() {
	y := 2

	x := 1
}
`,
		},
		{
			code: `
				if config.Params["tx_isolation"] == "" {
					delete(config.Params, "tx_isolation")
				}
			`,
			expected: "package main\n\nfunc main() {\n\tif config.Params[\"tx_isolation\"] == \"\" {\n\t\tdelete(config.Params, \"tx_isolation\")\n\t}\n\n\tx := 1\n}\n",
		},
	}

	for _, tt := range tests {
		file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			x := 1
		}
	`)})

		for _, assignments := range file.FindAssignments("x") {
			for _, assignment := range assignments {
				assignment.InsertBefore(codemod.Ast(tt.code))
			}
		}

		actual := string(file.SourceCode())

		check(t, tt.expected, actual)
	}
}

func Test_IfStmt_Remove(t *testing.T) {
	t.Parallel()

	file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			if 2 == 2 {
				println("hello")
			}
		}
	`)})

	for _, statements := range file.IfStatements() {
		for _, statement := range statements {
			if codemod.SourceCode(statement.Node.Cond) == "2 == 2" {
				statement.Remove()
			}
		}
	}

	expected :=
		`package main

func main() {

}
`

	actual := string(file.SourceCode())

	check(t, expected, actual)
}

func Test_IfStmt_InsertBefore(t *testing.T) {
	t.Parallel()

	file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			if 2 == 2 {
				println("hello")
			}
		}
	`)})

	for _, statements := range file.IfStatements() {
		for _, statement := range statements {
			if codemod.SourceCode(statement.Node.Cond) == "2 == 2" {
				statement.InsertBefore(codemod.Ast(`println("before if statement")`))
			}
		}
	}

	expected :=
		`package main

func main() {
	println("before if statement")
	if 2 == 2 {
		println("hello")
	}
}
`

	actual := string(file.SourceCode())

	check(t, expected, actual)
}

func Test_IfStmt_InsertAfter(t *testing.T) {
	t.Parallel()

	file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package main

		func main() {
			if 2 == 2 {
				println("hello")
			}
		}
	`)})

	for _, statements := range file.IfStatements() {
		for _, statement := range statements {
			if codemod.SourceCode(statement.Node.Cond) == "2 == 2" {
				statement.InsertAfter(codemod.Ast(`println("after if statement")`))
			}
		}
	}

	expected :=
		`package main

func main() {
	if 2 == 2 {
		println("hello")
	}
	println("after if statement")
}
`

	actual := string(file.SourceCode())

	check(t, expected, actual)
}

func Test_Package(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
	package main

	func main() {}
`)

	t.Run("returns package name", func(t *testing.T) {
		t.Parallel()

		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

		pkg := file.Package()

		assert.Equal(t, "main", pkg.Name())
	})

	t.Run("modifies package name", func(t *testing.T) {
		t.Parallel()

		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode, FilePath: "path"})

		pkg := file.Package()

		pkg.SetName("newpackagename")

		expected :=
			`package newpackagename

func main() {}
`

		assert.Equal(t, expected, string(file.SourceCode()))
	})
}

func Test_TraverseAst(t *testing.T) {
	t.Parallel()

	found := false

	file, _ := codemod.New(codemod.NewInput{SourceCode: []byte(`
		package bar 

		func z() {}
	`)})

	file.TraverseAst(func(node codemod.NodeWithParent) {
		if fun, ok := node.Node.(*ast.FuncDecl); ok {
			found = fun.Name.Name == "z"
		}
	})

	assert.True(t, found)
}

func Test_SourceFile_Path(t *testing.T) {
	t.Parallel()

	filePath := "src/services/user.go"

	file, _ := codemod.New(codemod.NewInput{
		SourceCode: []byte(`
		package main 
		func main() {}
		`),
		FilePath: filePath,
	})

	assert.Equal(t, filePath, file.FilePath)
}

func Test_SourceFile_Imports(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
	package main 

	import (
		"errors"
		"package_a"
		"package_b"
	)

	func main() {}
	`)

	t.Run("user is able to get file imports", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

		imports := file.Imports()

		assert.Equal(t, []string{"errors", "package_a", "package_b"}, imports.Paths())
	})

	t.Run("same import path is not added more than once", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

		imports := file.Imports()

		imports.Add("new_import")
		imports.Add("new_import")
		imports.Add("new_import")

		assert.Equal(t, []string{"errors", "package_a", "package_b", "new_import"}, imports.Paths())
	})

	t.Run("checks if file contains import path", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

		imports := file.Imports()

		assert.True(t, imports.Contains("errors"))
		assert.True(t, imports.Contains("package_a"))
		assert.True(t, imports.Contains("package_b"))
		assert.False(t, imports.Contains("package_c"))
	})

	t.Run("removes import path from file imports", func(t *testing.T) {
		file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

		imports := file.Imports()

		imports.Remove("package_a")

		assert.Equal(t, []string{"errors", "package_b"}, imports.Paths())
	})
}

func Test_SourceFile_SwitchStatements(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
		package p

		func f(x string) {
			switch x {
			case "a":
				println(1)
			case "b":
				println(2)
			default:
				println(3)
			}
		}
	`)

	file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

	scopedStatements := file.SwitchStatements()

	assert.Equal(t, 1, len(scopedStatements))

	for _, statements := range scopedStatements {
		for _, switchStmt := range statements {
			if codemod.SourceCode(switchStmt.Node.Tag) != "x" {
				continue
			}

			newSwitchStatementBody := make([]ast.Stmt, 0, len(switchStmt.Node.Body.List))

			for _, caseClause := range switchStmt.Node.Body.List {
				newSwitchStatementBody = append(newSwitchStatementBody, caseClause)

				if !strings.Contains(codemod.SourceCode(caseClause), `"b"`) {
					continue
				}

				newCaseClause := &ast.CaseClause{
					List: []ast.Expr{
						&ast.BasicLit{
							Value: `"c"`,
						},
					},
					Body: []ast.Stmt{
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: &ast.Ident{
									Name: "println",
								},
								Args: []ast.Expr{
									&ast.BasicLit{
										Kind:  token.INT,
										Value: "4",
									},
								},
							},
						},
					},
				}

				newSwitchStatementBody = append(newSwitchStatementBody, newCaseClause)
			}

			switchStmt.Node.Body.List = newSwitchStatementBody
		}
	}

	expected := `
package p

func f(x string) {
	switch x {
	case "a":
		println(1)
	case "b":
		println(2)
	case "c":
		println(4)
	default:
		println(3)
	}
}
`

	check(t, expected, string(file.SourceCode()))
}

func Test_Codemod_New(t *testing.T) {
	t.Parallel()

	t.Run("when Go file cannot be parsed", func(t *testing.T) {
		t.Run("returns error", func(t *testing.T) {
			fileMissingPackage := []byte(`
				func main() {}
			`)

			_, err := codemod.New(codemod.NewInput{SourceCode: fileMissingPackage})

			assert.NotNil(t, err)
		})
	})

	t.Run("when Go file can be parsed", func(t *testing.T) {
		t.Run("returns no error", func(t *testing.T) {
			sourceCode := []byte(`
			package main
			func main() {}
		`)

			_, err := codemod.New(codemod.NewInput{SourceCode: sourceCode})

			assert.Nil(t, err)
		})
	})
}

func Test_Example_UpdateNewRelicDataStoreCalls(t *testing.T) {
	t.Parallel()

	sourceCode := []byte(`
package repositories

import (
	"context"

	newrelic "github.com/newrelic/go-agent"
)

func Create(ctx context.Context) {
	s := newrelic.DatastoreSegment{
		Product:            newrelic.DatastoreMySQL,
		Collection:         "users",
		Operation:          "INSERT",
		ParameterizedQuery: "INSERT INTO users (name, age) VALUES ($1, $2)",
		QueryParameters: map[string]interface{}{
			"name": "Dracula",
			"age":  439,
		},
		Host:         "mysql-server-1",
		PortPathOrID: "3306",
		DatabaseName: "my_database",
	}

	s = startSegment(ctx, s)
	// ... make the datastore call
	s.End()
}
`)

	file, _ := codemod.New(codemod.NewInput{SourceCode: sourceCode})

	for scope, assignments := range file.FindAssignments("s") {
		s := assignments[0].Struct()

		args := []ast.Expr{
			s.Field("Collection").Expr,
			s.Field("Operation").Expr,
			s.Field("ParameterizedQuery").Expr,
		}

		newCall := &ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun:  &ast.Ident{Name: "StartDBSegment"},
						Args: args,
					},
					Sel: &ast.Ident{
						Name: "End",
					},
				},
				Args: nil, // End args,
			},
		}

		// remove s := newrelic.DatastoreSegment{...}
		assignments[0].Remove()

		// remove s = startSegment(...) with defer StartDBSegment(...).End()
		assignments[1].Replace(newCall)

		// remove s.End() call
		scope.FindCall("s.End").Remove()
	}

	expected :=
		`package repositories

import (
	"context"

	newrelic "github.com/newrelic/go-agent"
)

func Create(ctx context.Context) {
	defer StartDBSegment("users",
		"INSERT",
		"INSERT INTO users (name, age) VALUES ($1, $2)").End()

	// ... make the datastore call

}
`

	actual := string(file.SourceCode())

	check(t, expected, actual)
}
