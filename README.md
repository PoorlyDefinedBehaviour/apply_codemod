Inspired by [Facebook's codemod](https://github.com/facebookarchive/codemod) and [Facebook's jscodeshift](https://github.com/facebook/jscodeshift)

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

func (Int) expr() {}


type BinaryOperation struct {
	Left     Expr
	Operator Operator
	Right    Expr
}

func (BinaryOperation) expr() {}
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
travese the `struct` that represents the source code and interpret it

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
  // we travese left and right because
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

# Examples

```go

package main

import (
	"github.com/poorlydefinedbehaviour/apply_codemod/src/apply"
	"github.com/poorlydefinedbehaviour/apply_codemod/src/codemod"
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
}

func main() {
 	sourceCode := []byte(`
	package main

	import "errors"

	var errSomething = errors.New("oops")

	func foo() error {
		return errors.Wrapf(errSomething, "some context")
	}

	func main() {}
	`)

	file, err := codemod.New(codemod.NewInput{SourceCode: sourceCode})
  if err != nil {
    panic(err)
  }

  rewriteErrorsWrapfToFmtErrorf(file)

  fmt.Println(file.SourceCode())
>> package main
>>
>> import (
>>  "errors"
>>  "fmt"
>> )
>>
>> var errSomething = errors.New("oops")
>>
>> func foo() error {
>>  return fmt.Errorf("some context: %w", errSomething)
>> }
>>
>> func main() {}
}
```
