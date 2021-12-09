Inspired by [Facebook's codemod](https://github.com/facebookarchive/codemod) and [Facebook's jscodeshift](https://github.com/facebook/jscodeshift)

# Check out the [examples](https://github.com/PoorlyDefinedBehaviour/apply_codemod/tree/main/examples)

# What is a codemod?

According to Facebook, codemod is a tool/library to assist you with large-scale codebase refactors that can be partially automated but still require human oversight and occasional intervention.

For this project, codemod is defined as a function that modifies the Go abstract syntax tree to make automated changes to Go codebases.

# What is this project?

This project provides helper functions to find specific Go abstract syntax tree nodes, inspect and modify them.

It also let's you apply one or more codemod to several Github repositories at once.

# What is an abstract syntax tree?

An abstract syntax tree is a tree representation of the source code.

We could represent the expression

```go
2 + 2 + 2
```

as a `struct` with one operator and two operands

```go
type Operator rune

const (
  PLUS  Operator = '+'
  MINUS Operator = '-'
  MUL   Operator = '*'
  DIV   Operator = '/'
)

type Expr interface {
  expr()
}

type Int struct {
  Value int
}

func (*Int) expr() {}

type BinaryOperation struct {
  Left     Expr
  Operator Operator
  Right    Expr
}

func (*BinaryOperation) expr() {}
```

here is an abstract syntax tree representing the expression `2 + 2 + 2`

```go
expr := &BinaryOperation{
  Left: &BinaryOperation{
    Left:     &Int{Value: 2},
    Operator: PLUS,
    Right:    &Int{Value: 2},
  },
  Operator: PLUS,
  Right: &Int{Value: 2},
}
```

and if we wanted to produce a value out of the expression we could
traverse the `struct` that represents the source code and interpret it

```go
func eval(expr Expr) int {
  switch expr := expr.(type) {
  case *BinaryOperation:
    switch expr.Operator {
    case PLUS:
      return eval(expr.Left) + eval(expr.Right)
    case MINUS:
      return eval(expr.Left) - eval(expr.Right)
    case MUL:
      return eval(expr.Left) * eval(expr.Right)
    case DIV:
      return eval(expr.Left) + eval(expr.Right)
    }
  case *Int:
    return expr.Value
  }

  panic(fmt.Sprintf("unknown expr %T", expr))
}


eval(expr) // 6
```

# Intuition for codemods

We are still using the expression `2 + 2 + 2` represented by the abstract syntax tree

```go
expr := &BinaryOperation{
  Left: &BinaryOperation{
    Left:     &Int{Value: 2},
    Operator: PLUS,
    Right:    &Int{Value: 2},
  },
  Operator: PLUS,
  Right: &Int{Value: 2},
}
```

What if we wanted to replace every addition operation in our codebase that has 1000 files by multiplication?

We could do it manually or... write a function that does it for us.

Here's a function that does this

```go
func additionToMultiplication(expr Expr) {
  // We only care about binary operations,
  // if the expression is of any other type,
  // Int for example, we just ignore it.
  op, ok := expr.(*BinaryOperation)
  if !ok {
    return
  }

  // Whenever we find a binary operation
  // that has + as its operator,
  // we change the operator to *.
  if op.Operator == PLUS {
    op.Operator == MUL
  }

  // Since the expression is represented by a tree
  // we traverse left and right because
  // we may find another binary operation.
  additionToMultiplication(op.Left)
  additionToMultiplication(op.Right)
}
```

if we pass `expr` to `additionToMultiplication`, we will get

```go
expr := &BinaryOperation{
  Left: &BinaryOperation{
    Left:     &Int{Value: 2},
    Operator: MUL,
    Right:    &Int{Value: 2},
  },
  Operator: MUL,
  Right: &Int{Value: 2},
}
```

and if we evalute this expression, we will get

```go
eval(expr) // 8
```

The idea here is that you can do the same thing for the Go abstract syntax tree.

# Install

```terminal
go get github.com/poorlydefinedbehaviour/apply_codemod
```

# Example

## Applying codemods to remote repositories

```go
import (
  "github.com/poorlydefinedbehaviour/apply_codemod/src/apply"
  "github.com/poorlydefinedbehaviour/apply_codemod/src/codemod"
  "go/ast"
)
// Goes from:
//
// errors.Wrapf(...)
//
// to
//
// fmt.Errorf(...)
func transform(file *codemod.SourceFile) {
  scopedCalls := file.FunctionCalls()

  for _, calls := range scopedCalls {
    for _, call := range calls {
      if call.FunctionName() != "errors.Wrapf" {
        continue
      }

      args := call.Node.Args

      args[0], args[len(args)-1] = args[len(args)-1], args[0]

      args[0].(*ast.BasicLit).Value =
        codemod.Quote(codemod.Unquote(args[0].(*ast.BasicLit).Value) + ": %w")

      call.Node.Fun = &ast.SelectorExpr{
        X:   &ast.Ident{Name: "fmt"},
        Sel: &ast.Ident{Name: "Errorf"},
      }
    }
  }

  file.Imports().Add("fmt")
}

func main() {
  codemods := []apply.Target{
    {
      Repo: apply.Repository{
        AccessToken: "github_access_token",
        URL:         "https://github.com/PoorlyDefinedBehaviour/repo_1",
        Branch:      "main",
      },
      Codemods: []apply.Codemod{
        {
          Description: "replaces errors.Wrapf with fmt.Errorf",
          Transform: transform,
        }
      },
    },
    {
      Repo: apply.Repository{
        AccessToken: "github_access_token",
        URL:         "https://github.com/PoorlyDefinedBehaviour/repo_2",
        Branch:      "development",
      },
      Codemods: []apply.Codemod{
        {
          Description: "replaces errors.Wrapf with fmt.Errorf",
          Transform: transform,
        }
      },
    },
  }

  err := apply.Codemods(codemods)
  if err != nil {
    panic(err)
  }
}
```

## Applying codemods to local directory

We can apply codemods to local directories by calling `apply.Locally` in the code
and running it with the `-dir` flag:

```terminal
go run main.go -dir=absolute/path/to/repository/in/my/computer
```

```go
// main.go

package main

func rewriteErrorsWrapfToFmtErrorf(code *codemod.SourceFile) {
  ...
}

func updateNewrelicDatastoreCalls(code *codemod.SourceFile) {
  ...
}

func updateTransactionIsolationParameter(code *codemod.SourceFile) {
  ...
}

func addsCodeOwnersFile(code *codemod.Project) {
  ...
}

func main() {
  apply.Locally(
		[]apply.Codemod{
			{
				Description: "replaces errors.Wrapf with fmt.Errorf",
				Transform:   rewriteErrorsWrapfToFmtErrorf,
			},
			{
				Description: "updates new relic DatastoreSegment calls to the new version",
				Transform:   updateNewrelicDatastoreCalls,
			},
			{
				Description: "passes tx_isolation instead of transaction_isolation to MySQL",
				Transform:   updateTransactionIsolationParameter,
			},
      {
				Description: "adds CODEOWNERS file",
				Transform: addsCodeOwnersFile,
			}
	  },
  )
}
```
