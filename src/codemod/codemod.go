package codemod

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"

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

func SourceCode(node ast.Node) string {
	var buffer bytes.Buffer

	err := format.Node(&buffer, token.NewFileSet(), node)
	if err != nil {
		panic(err)
	}

	return buffer.String()
}

func FromSourceCode(sourceCode string) ast.Expr {
	ast, err := parser.ParseExpr(sourceCode)
	if err != nil {
		panic(errors.WithStack(err))
	}

	return ast
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

func Quote(s string) string {
	return fmt.Sprintf(`"%s"`, s)
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

func (imports *Imports) Some(predicate func(string) bool) bool {
	for _, path := range imports.Paths() {
		if predicate(path) {
			return true
		}
	}

	return false
}

func (imports *Imports) Add(importPath string) {
	*imports.specs = append(*imports.specs, &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: Quote(importPath),
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
	Parent NodeWithParent
	Node   *ast.CallExpr
}

type Replacement interface {
	CallExpr() *ast.CallExpr
}

func (call *FunctionCall) Replace(node ast.Expr) {
	funDecl := call.Parent.FindUpstreamNode(&ast.FuncDecl{})

	block := funDecl.Node.(*ast.FuncDecl).Body

	for i := 0; i < len(block.List); i++ {
		switch value := block.List[i].(type) {
		case *ast.ExprStmt:
			callExpr, ok := value.X.(*ast.CallExpr)

			if ok &&
				reflect.DeepEqual(callExpr.Fun, call.Node.Fun) &&
				reflect.DeepEqual(callExpr.Args, call.Node.Args) {
				block.List[i] = &ast.ExprStmt{X: node}
			}

		case *ast.ReturnStmt:
			for i := 0; i < len(value.Results); i++ {
				callExpr, ok := value.Results[i].(*ast.CallExpr)

				if ok &&
					reflect.DeepEqual(callExpr.Fun, call.Node.Fun) &&
					reflect.DeepEqual(callExpr.Args, call.Node.Args) {
					value.Results[i] = node.(ast.Expr)
				}
			}
		}
	}
}

func (call *FunctionCall) Remove() {
	if blockStmt := call.Parent.FindUpstreamNode(&ast.BlockStmt{}); blockStmt != nil {
		block := blockStmt.Node.(*ast.BlockStmt)

		list := make([]ast.Stmt, 0)

		for _, stmt := range block.List {
			exprStmt, ok := stmt.(*ast.ExprStmt)

			if ok {
				callExpr, ok := exprStmt.X.(*ast.CallExpr)
				if ok &&
					reflect.DeepEqual(callExpr.Fun, call.Node.Fun) &&
					reflect.DeepEqual(callExpr.Args, call.Node.Args) {
					continue
				}
			}

			list = append(list, stmt)
		}

		block.List = list
	}
}

// TODO: refactor me to use SourceFile.FunctionCalls()
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
					out[scope] = append(out[scope], FunctionCall{Node: value, Parent: parent})
				}
			}

			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

			return true
		},
	)

	return out
}

func (code *SourceFile) FunctionCalls() map[Scope][]FunctionCall {
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
				out[scope] = append(out[scope], FunctionCall{Node: value, Parent: parent})
			}

			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

			return true
		},
	)

	return out
}

type Function struct {
	Parent NodeWithParent
	Node   *ast.FuncDecl
}

func (code *SourceFile) Functions() []Function {
	out := make([]Function, 0)

	var parent NodeWithParent

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				out = append(out, Function{Node: value, Parent: parent})
			}

			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

			return true
		},
	)

	return out
}

type Map struct {
	Expr NodeWithParent
}

func (m *Map) Has(key string) bool {
	composite := m.Expr.Node.(*ast.CompositeLit)

	for _, element := range composite.Elts {
		if Unquote(element.(*ast.KeyValueExpr).Key.(*ast.BasicLit).Value) == key {
			return true
		}
	}

	return false
}

func (m *Map) RenameKey(currentKeyValue string, newKeyValue string) {
	composite := m.Expr.Node.(*ast.CompositeLit)

	for _, element := range composite.Elts {
		key := element.(*ast.KeyValueExpr).Key.(*ast.BasicLit)

		if Unquote(key.Value) != currentKeyValue {
			continue
		}

		key.Value = Quote(newKeyValue)
	}
}

func (code *SourceFile) FindMapLiteral(mapType string) (*Scope, *Map) {
	for scope, literals := range code.FindMapLiterals(mapType) {
		return &scope, &literals[0]
	}

	return nil, nil
}

func (code *SourceFile) FindMapLiterals(mapType string) map[Scope][]Map {
	out := make(map[Scope][]Map)
	var parent NodeWithParent
	var scope Scope

	mapTypeExpr, err := parser.ParseExpr(mapType)
	if err != nil {
		panic(errors.Wrapf(err, "invalid map type: %s", mapType))
	}

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				scope = Scope{fun: value}

			case *ast.CompositeLit:
				typ := value.Type.(*ast.MapType)

				if typ.Key.(*ast.Ident).Name == mapTypeExpr.(*ast.MapType).Key.(*ast.Ident).Name &&
					typ.Value.(*ast.Ident).Name == mapTypeExpr.(*ast.MapType).Value.(*ast.Ident).Name {
					p := parent
					out[scope] = append(out[scope], Map{Expr: NodeWithParent{Parent: &p, Node: node}})
				}
			}

			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

			return true
		},
	)

	return out
}

type IfStmt NodeWithParent

func (code *SourceFile) FindIfStatements() map[Scope][]IfStmt {
	out := make(map[Scope][]IfStmt)
	var parent NodeWithParent
	var scope Scope

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				scope = Scope{fun: value}

			case *ast.IfStmt:
				p := parent
				out[scope] = append(out[scope], IfStmt(NodeWithParent{Parent: &p, Node: node}))

			}

			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
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
	Parent *NodeWithParent
	Node   ast.Node
}

func (node *NodeWithParent) FindUpstreamNode(targetNode ast.Node) *NodeWithParent {
	current := node

	for {
		if reflect.TypeOf(current.Node) == reflect.TypeOf(targetNode) {
			return current
		}

		if current.Parent == nil {
			return nil
		}

		current = current.Parent
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
				call = &FunctionCall{Node: callExpr, Parent: parent}
				return false
			}

			if node != nil {
				p := parent
				parent = NodeWithParent{
					Parent: &p,
					Node:   node,
				}
			}

			return true
		},
	)

	return call
}

type Assignment struct {
	Parent NodeWithParent
	Stmt   *ast.AssignStmt
	Lhs    []*ast.Ident
	Rhs    []ast.Expr
}

func (assignment *Assignment) Remove() {
	if nodeWithParent := assignment.Parent.FindUpstreamNode(&ast.FuncDecl{}); nodeWithParent != nil {
		funDecl := nodeWithParent.Node.(*ast.FuncDecl)

		stmts := make([]ast.Stmt, 0)
		for _, stmt := range funDecl.Body.List {
			if reflect.DeepEqual(stmt, assignment.Stmt) {
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

type Value struct {
	Expr ast.Expr
}

func (struct_ *Struct) Field(key string) Value {
	for _, element := range struct_.literal.Elts {
		expr := element.(*ast.KeyValueExpr)
		if expr.Key.(*ast.Ident).Name == key {
			return Value{Expr: expr.Value}
		}
	}

	panic(fmt.Sprintf("struct %+v doesn't have a property called %+v", struct_, key))
}

func (assignment *Assignment) Struct() Struct {
	composite := assignment.Rhs[0].(*ast.CompositeLit)
	return Struct{literal: composite}
}

func (assignment *Assignment) Replace(node ast.Stmt) {
	if funDecl := assignment.Parent.FindUpstreamNode(&ast.FuncDecl{}); funDecl != nil {

		block := funDecl.Node.(*ast.FuncDecl).Body

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
								Parent: parent,
								Stmt:   stmt,
								Lhs:    lhss,
								Rhs:    stmt.Rhs,
							})
						}
					}

				}
			}
		}

		if node != nil {
			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}
		}

		return true
	},
	)

	return out
}
