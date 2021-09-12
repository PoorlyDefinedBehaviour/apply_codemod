package codemod

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

type SourceFile struct {
	fileSet *token.FileSet
	file    *ast.File
}

func New(sourceCode []byte) *SourceFile {
	// a file set represents a set of source files
	fileSet := token.NewFileSet()

	// parser.ParseComments tells the parser to include comments
	ast, err := parser.ParseFile(fileSet, "", sourceCode, parser.ParseComments)
	if err != nil {
		panic(errors.WithStack(err))
	}

	return &SourceFile{fileSet: fileSet, file: ast}
}

func NewFunctionCall(selector string, values []Value) FunctionCall {
	args := make([]ast.Expr, 0, len(values))

	for _, value := range values {
		args = append(args, *value.expr)
	}

	parts := strings.Split(selector, ".")

	var expr *ast.CallExpr

	if len(parts) == 1 {
		expr = &ast.CallExpr{
			Fun: &ast.Ident{
				Name: parts[0],
			},
			Args: args,
		}

	} else {
		expr = &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.Ident{
					Name: parts[0],
				},
				Sel: &ast.Ident{
					Name: parts[1],
				},
			},
			Args: args,
		}

	}

	return FunctionCall{fun: expr}
}

type Source struct {
	expr ast.Expr
}

func (source *Source) CallExpr() *ast.CallExpr {
	return source.expr.(*ast.CallExpr)
}

func Placeholders(count int) string {
	placeholders := make([]string, 0, count)

	for i := 0; i < count; i++ {
		placeholders = append(placeholders, "%s")
	}

	return strings.Join(placeholders, ",")
}

func FromSourceCode(sourceCode string, args ...interface{}) *Source {
	ast, err := parser.ParseExpr(fmt.Sprintf(sourceCode, args...))
	if err != nil {
		panic(errors.WithStack(err))
	}

	return &Source{expr: ast}
}

func (code *SourceFile) SourceCode() []byte {
	buffer := bytes.Buffer{}

	err := format.Node(&buffer, code.fileSet, code.file)
	if err != nil {
		panic(errors.WithStack(err))
	}

	return buffer.Bytes()
}

func Unquote(s string) string {
	return s[1 : len(s)-1]
}

func getCallExprLiteral(cursor *ast.CallExpr) string {
	selector, ok := cursor.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%s.%s", identifier.Name, selector.Sel.Name)
}

type Imports struct {
	specs *[]ast.Spec
}

func (imports *Imports) Paths() []string {
	out := make([]string, 0, len(*imports.specs))

	for _, spec := range *imports.specs {
		import_ := spec.(*ast.ImportSpec)

		out = append(out, Unquote(import_.Path.Value))
	}

	return out
}

func (imports *Imports) Contains(target string) bool {
	for _, path := range imports.Paths() {
		if path == target {
			return true
		}
	}

	return false
}

func (imports *Imports) Add(importPath string) {
	*imports.specs = append(*imports.specs, &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%s"`, importPath),
		},
	})
}

func (code *SourceFile) Imports() Imports {
	var specs *[]ast.Spec

	ast.Inspect(code.file, func(node ast.Node) bool {
		decl, ok := node.(*ast.GenDecl)
		if ok && decl.Tok == token.IMPORT {
			specs = &decl.Specs
			return false
		}
		return true
	})

	return Imports{specs: specs}
}

type FunctionCall struct {
	parent NodeWithParent
	fun    *ast.CallExpr
}

func (call *FunctionCall) Args() Values {
	out := make(Values, 0, len(call.fun.Args))

	for i := 0; i < len(call.fun.Args); i++ {
		out = append(out, Value{expr: &call.fun.Args[i]})
	}

	return out
}

type Replacement interface {
	CallExpr() *ast.CallExpr
}

func (call *FunctionCall) Replace(node ast.Stmt) {
	funDecl := call.parent.FindUpstreamNode(&ast.FuncDecl{})

	block := funDecl.node.(*ast.FuncDecl).Body

	for i := 0; i < len(block.List); i++ {
		switch value := block.List[i].(type) {
		case *ast.ExprStmt:
			callExpr, ok := value.X.(*ast.CallExpr)

			if ok &&
				reflect.DeepEqual(callExpr.Fun, call.fun.Fun) &&
				reflect.DeepEqual(callExpr.Args, call.fun.Args) {
				block.List[i] = node
			}

		case *ast.ReturnStmt:
			for i := 0; i < len(value.Results); i++ {
				callExpr, ok := value.Results[i].(*ast.CallExpr)

				if ok &&
					reflect.DeepEqual(callExpr.Fun, call.fun.Fun) &&
					reflect.DeepEqual(callExpr.Args, call.fun.Args) {
					block.List[i] = node
				}
			}
		}
	}
}

func (call *FunctionCall) Remove() {
	if blockStmt := call.parent.FindUpstreamNode(&ast.BlockStmt{}); blockStmt != nil {
		block := blockStmt.node.(*ast.BlockStmt)

		list := make([]ast.Stmt, 0)

		for _, stmt := range block.List {
			exprStmt, ok := stmt.(*ast.ExprStmt)

			if ok {
				callExpr, ok := exprStmt.X.(*ast.CallExpr)
				if ok &&
					reflect.DeepEqual(callExpr.Fun, call.fun.Fun) &&
					reflect.DeepEqual(callExpr.Args, call.fun.Args) {
					continue
				}
			}

			list = append(list, stmt)
		}

		block.List = list
	}
}

func (code *SourceFile) FindCalls(target string) map[Scope][]FunctionCall {
	out := make(map[Scope][]FunctionCall)

	var parent NodeWithParent
	var scope Scope

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				scope = Scope{fun: value}

			case *ast.CallExpr:
				if getCallExprLiteral(value) == target {
					out[scope] = append(out[scope], FunctionCall{fun: value, parent: parent})
				}
			}

			p := parent
			parent = NodeWithParent{
				parent: &p,
				node:   node,
			}

			return true
		},
	)

	return out
}

type Scope struct {
	fun *ast.FuncDecl
}

type NodeWithParent struct {
	parent *NodeWithParent
	node   ast.Node
}

func (node *NodeWithParent) FindUpstreamNode(targetNode ast.Node) *NodeWithParent {
	current := node

	for {
		if reflect.TypeOf(current.node) == reflect.TypeOf(targetNode) {
			return current
		}

		if current.parent == nil {
			return nil
		}

		current = current.parent
	}
}

func (scope *Scope) FindCall(selector string) *FunctionCall {
	var call *FunctionCall

	var parent NodeWithParent

	// TODO: remove duplication
	ast.Inspect(
		scope.fun,
		func(node ast.Node) bool {
			callExpr, ok := node.(*ast.CallExpr)
			if ok && getCallExprLiteral(callExpr) == selector {
				call = &FunctionCall{fun: callExpr, parent: parent}
				return false
			}

			if node != nil {
				p := parent
				parent = NodeWithParent{
					parent: &p,
					node:   node,
				}
			}

			return true
		},
	)

	return call
}

type Assignment struct {
	parent NodeWithParent
	stmt   *ast.AssignStmt
	lhs    []*ast.Ident
	rhs    []ast.Expr
}

func (assignment *Assignment) Remove() {
	if nodeWithParent := assignment.parent.FindUpstreamNode(&ast.FuncDecl{}); nodeWithParent != nil {
		funDecl := nodeWithParent.node.(*ast.FuncDecl)

		stmts := make([]ast.Stmt, 0)
		for _, stmt := range funDecl.Body.List {
			if reflect.DeepEqual(stmt, assignment.stmt) {
				continue
			}

			stmts = append(stmts, stmt)
		}

		funDecl.Body.List = stmts
	}
}

type Struct struct {
	literal *ast.CompositeLit
}

func (struct_ *Struct) SourceCode() string {
	var buffer bytes.Buffer

	format.Node(&buffer, token.NewFileSet(), struct_.literal)

	return buffer.String()
}

type Value struct {
	expr *ast.Expr
}

type Values []Value

func (values Values) Unwrap() []ast.Expr {
	out := make([]ast.Expr, 0, len(values))

	for _, value := range values {
		out = append(out, *value.expr)
	}

	return out
}

func (arg *Value) IsString() bool {
	literal, ok := (*arg.expr).(*ast.BasicLit)
	return ok && literal.Kind == token.STRING
}

func (arg *Value) String() string {
	literal := (*arg.expr).(*ast.BasicLit)
	return Unquote(literal.Value)
}

func (arg *Value) SetString(value string) {
	literal := (*arg.expr).(*ast.BasicLit)
	literal.Value = fmt.Sprintf(`"%s"`, value)
}

func (struct_ *Struct) Field(key string) Value {
	for _, element := range struct_.literal.Elts {
		expr := element.(*ast.KeyValueExpr)
		if expr.Key.(*ast.Ident).Name == key {
			return Value{expr: &expr.Value}
		}
	}

	panic(fmt.Sprintf("struct %s doesn't have a property called %s", struct_.SourceCode(), key))
}

func (assignment *Assignment) Struct() Struct {
	composite := assignment.rhs[0].(*ast.CompositeLit)
	return Struct{literal: composite}
}

func (assignment *Assignment) Replace(node ast.Stmt) {
	if funDecl := assignment.parent.FindUpstreamNode(&ast.FuncDecl{}); funDecl != nil {

		block := funDecl.node.(*ast.FuncDecl).Body

		for i := 0; i < len(block.List); i++ {
			assignStmt, ok := block.List[i].(*ast.AssignStmt)

			if ok &&
				reflect.DeepEqual(assignStmt.Lhs, assignStmt.Lhs) &&
				reflect.DeepEqual(assignStmt.Rhs, assignStmt.Rhs) {
				block.List[i] = node
			}
		}
	}
}

func (code *SourceFile) FindAssignments(target string) map[Scope][]Assignment {
	out := make(map[Scope][]Assignment)

	var parent NodeWithParent

	var scope Scope

	ast.Inspect(code.file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.FuncDecl:
			scope = Scope{fun: value}

		case *ast.BlockStmt:
			for _, statement := range value.List {
				stmt, ok := statement.(*ast.AssignStmt)
				if ok {
					lhss := make([]*ast.Ident, 0, len(stmt.Lhs))

					for _, expr := range stmt.Lhs {
						ident := expr.(*ast.Ident)
						lhss = append(lhss, ident)
					}

					for _, ident := range lhss {
						if ident.Name == target {
							out[scope] = append(out[scope], Assignment{
								parent: parent,
								stmt:   stmt,
								lhs:    lhss,
								rhs:    stmt.Rhs,
							})
						}
					}

				}
			}
		}

		if node != nil {
			p := parent
			parent = NodeWithParent{
				parent: &p,
				node:   node,
			}
		}

		return true
	},
	)

	return out
}
