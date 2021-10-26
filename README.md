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

```go
func mod(file *codemod.SourceFile)  {
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
				fun, ok := expr.Fun.(*ast.SelectorExpr); ok {
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

// Looking for type declarations that contain context.Context
//
// Example:
//
// type Foo interface {
//   f(x int64, ctx context.Context) error
// }
for _, typeDecl := range file.TypeDeclarations() {
	for _, method := range typeDecl.Methods() {
		params := method.Params()

		for i, param := range params {
      // Whenever we find context.Context in a type declaration
      // move to it position 0.
      //
      // The foo interface would become:
      //
      // type Foo interface {
      //  f(ctx context.Context, x int64) error
      // }
			if codemod.SourceCode(param.Type) == "context.Context" {
				params[0], params[i] = params[i], params[0]
			}
		}
	}
}

}

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

mod(file)

fmt.Println(file.SourceCode())
>> package main
>>
>> import "context"
>>
>> type UserService interface {
>>  DoSomething(context.Context, int64) error
>> }
>>
>> func buz(ctx context.Context, userID int64) error {
>> 	return nil
>> }
>>
>> func baz(context context.Context, userID int64) error {
>> 	return buz(context, userID)
>> }
>>
>> func foo(ctx context.Context, userID int64) error {
>> 	err := baz(ctx, userID)
>>  if err != nil {
>> 	  return err
>>  }
>>  return nil
>> }
>>
>> func main() {
>>  _ = foo(context.Background(), 1)
>> }
```
