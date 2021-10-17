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
	"time"

	"github.com/pkg/errors"
)

type Project struct {
	ProjectRoot string
}

type SourceFile struct {
	fileSet     *token.FileSet
	file        *ast.File
	ProjectRoot string
	FilePath    string
}

type NewInput struct {
	ProjectRoot string
	SourceCode  []byte
	FilePath    string
}

func New(input NewInput) *SourceFile {
	// a file set represents a set of source files
	fileSet := token.NewFileSet()

	// parser.ParseComments tells the parser to include comments
	ast, err := parser.ParseFile(fileSet, "", input.SourceCode, parser.ParseComments)
	if err != nil {
		panic(errors.WithStack(err))
	}

	return &SourceFile{
		fileSet:     fileSet,
		file:        ast,
		FilePath:    input.FilePath,
		ProjectRoot: input.ProjectRoot,
	}
}

func NormalizeString(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}

func insertAfter(node NodeWithParent, newNode ast.Node) {
	block := node.Parent.FindUpstreamNode(&ast.BlockStmt{}).Node.(*ast.BlockStmt)

	newList := make([]ast.Stmt, 0, len(block.List))

	for _, stmt := range block.List {
		newList = append(newList, stmt)

		if stmt == node.Node {
			newList = append(newList, newNode.(ast.Stmt))
		}
	}

	block.List = newList
}

func insertBefore(node NodeWithParent, newNode ast.Node) {
	block := node.Parent.FindUpstreamNode(&ast.BlockStmt{}).Node.(*ast.BlockStmt)

	newList := make([]ast.Stmt, 0, len(block.List))

	for _, stmt := range block.List {
		if stmt == node.Node {
			newList = append(newList, newNode.(ast.Stmt))
		}

		newList = append(newList, stmt)
	}

	block.List = newList
}

func remove(node NodeWithParent) {
	blockStmt := node.Parent.FindUpstreamNode(&ast.BlockStmt{})

	if blockStmt == nil {
		return
	}

	block := blockStmt.Node.(*ast.BlockStmt)

	list := make([]ast.Stmt, 0)

	for _, stmt := range block.List {
		if stmt == node.Node {
			continue
		}

		list = append(list, stmt)
	}

	block.List = list
}

func SourceCode(node ast.Node) string {
	var buffer bytes.Buffer

	err := format.Node(&buffer, token.NewFileSet(), node)
	if err != nil {
		panic(err)
	}

	return buffer.String()
}

var x int = 1

func zeroPositionInformation(value reflect.Value) {
	if !value.IsValid() || value.IsZero() {
		return
	}

	// TODO: something is causing infinite recursion in this function
	// this is work around so we can test something that depends on this
	if x > 1000 {
		return
	}
	x++

	if value.Type().Name() == "Pos" {
		value.Set(reflect.Zero(value.Type()))
	}

	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !value.IsNil() {
			zeroPositionInformation(value.Elem())
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			zeroPositionInformation(value.Field(i))
		}
	case reflect.Array, reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			zeroPositionInformation(value.Index(i))
		}
	}
}

func Ast(sourceCode string) ast.Node {
	packageName := fmt.Sprintf("package_name_%d\n", time.Now().Nanosecond())
	functionName := fmt.Sprintf("function_name_%d", time.Now().Nanosecond())

	if !strings.Contains(sourceCode, "package") {
		if !strings.Contains(sourceCode, "func") {
			sourceCode = fmt.Sprintf("func %s() { %s }", functionName, sourceCode)
		}

		sourceCode = fmt.Sprintf("package %s %s", packageName, sourceCode)
	}

	sourceFile := New(NewInput{SourceCode: []byte(sourceCode)})

	var node ast.Node

	body := sourceFile.file.Decls[0].(*ast.FuncDecl).Body

	if len(body.List) > 1 {
		node = body
	} else {
		node = body.List[0]
	}

	zeroPositionInformation(reflect.ValueOf(node))

	return node
}

func Unquote(s string) string {
	return s[1 : len(s)-1]
}

func Quote(s string) string {
	return fmt.Sprintf(`"%s"`, s)
}

func (code *SourceFile) SourceCode() []byte {
	buffer := bytes.Buffer{}

	err := format.Node(&buffer, code.fileSet, code.file)
	if err != nil {
		panic(errors.WithStack(err))
	}

	return buffer.Bytes()
}

func (code *SourceFile) TraverseAst(f func(NodeWithParent)) {
	parent := NodeWithParent{
		Node: code.file,
	}

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

			f(parent)

			return true
		},
	)
}

type Package struct {
	Identifier *ast.Ident
}

func (pkg *Package) Name() string {
	return pkg.Identifier.Name
}

func (pkg *Package) SetName(name string) {
	pkg.Identifier.Name = name
}

func (code *SourceFile) Package() Package {
	return Package{Identifier: code.file.Name}
}

type Imports struct {
	specs *[]ast.Spec
}

func (imports *Imports) Paths() []string {
	out := make([]string, 0, len(*imports.specs))

	for _, spec := range *imports.specs {
		importStatement := spec.(*ast.ImportSpec)

		out = append(out, Unquote(importStatement.Path.Value))
	}

	return out
}

func (imports *Imports) Contains(importPath string) bool {
	for _, path := range imports.Paths() {
		if path == importPath {
			return true
		}
	}

	return false
}

func (imports *Imports) Add(importPath string) {
	for _, path := range imports.Paths() {
		if path == importPath {
			return
		}
	}

	*imports.specs = append(*imports.specs, &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: Quote(importPath),
		},
	})
}

func (imports *Imports) Remove(importPath string) {
	specs := make([]ast.Spec, 0)

	for _, spec := range *imports.specs {
		if Unquote(spec.(*ast.ImportSpec).Path.Value) == importPath {
			continue
		}

		specs = append(specs, spec)
	}

	*imports.specs = specs
}

func (code *SourceFile) Imports() *Imports {
	var specs *[]ast.Spec

	ast.Inspect(code.file, func(node ast.Node) bool {
		decl, ok := node.(*ast.GenDecl)
		if ok && decl.Tok == token.IMPORT {
			specs = &decl.Specs
			return false
		}
		return true
	})

	return &Imports{specs: specs}
}

type FunctionCall struct {
	Parent NodeWithParent
	Node   *ast.CallExpr
}

func (call *FunctionCall) InsertAfter(node ast.Node) {
	insertAfter(NodeWithParent{Parent: &call.Parent, Node: call.Parent.Node}, node)
}

func (call *FunctionCall) InsertBefore(node ast.Node) {
	insertBefore(NodeWithParent{Parent: &call.Parent, Node: call.Parent.Node}, node)
}

func (call *FunctionCall) Remove() {
	remove(NodeWithParent{Parent: &call.Parent, Node: call.Parent.Node})
}

func (call *FunctionCall) FunctionName() string {
	return SourceCode(call.Node.Fun)
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

type TypeDeclaration struct {
	Parent NodeWithParent
	Node   *ast.TypeSpec
}

func (typeDecl *TypeDeclaration) IsInterface() bool {
	_, ok := typeDecl.Node.Type.(*ast.InterfaceType)
	return ok
}

func (typeDecl *TypeDeclaration) IsStruct() bool {
	_, ok := typeDecl.Node.Type.(*ast.StructType)
	return ok
}

func (typeDecl *TypeDeclaration) IsTypeAlias() bool {
	return !typeDecl.IsInterface() && !typeDecl.IsStruct()
}

type Method struct {
	Node ast.Node
}

func (method *Method) Params() []*ast.Field {
	return method.Node.(*ast.Field).Type.(*ast.FuncType).Params.List
}

func (method *Method) Name() string {
	switch node := method.Node.(type) {
	case *ast.FuncDecl:
		return node.Name.Name
	case *ast.Field:
		return node.Names[0].Name
	default:
		panic(fmt.Sprintf("node is not a method: %+v", method.Node))
	}
}

func (typeDecl *TypeDeclaration) Methods() []Method {
	out := make([]Method, 0)

	if typeDecl.IsInterface() {
		node := typeDecl.Node.Type.(*ast.InterfaceType)

		for _, method := range node.Methods.List {
			out = append(out, Method{Node: method})
		}
	}

	if typeDecl.IsStruct() || typeDecl.IsTypeAlias() {
		file := typeDecl.Parent.FindUpstreamNode(&ast.File{})

		for _, decl := range file.Node.(*ast.File).Decls {
			if funDecl, ok := decl.(*ast.FuncDecl); ok {
				if funDecl.Recv == nil {
					continue
				}

				for _, receiver := range funDecl.Recv.List {
					if pointerReceiver, ok := receiver.Type.(*ast.StarExpr); ok {
						if pointerReceiver.X.(*ast.Ident).Name == typeDecl.Node.Name.Name {
							out = append(out, Method{Node: funDecl})
						}
					}

					if byCopyReceiver, ok := receiver.Type.(*ast.Ident); ok {
						if byCopyReceiver.Name == typeDecl.Node.Name.Name {
							out = append(out, Method{Node: funDecl})
						}
					}
				}
			}
		}
	}

	return out
}

func (code *SourceFile) TypeDeclarations() []TypeDeclaration {
	out := make([]TypeDeclaration, 0)

	parent := NodeWithParent{
		Parent: nil,
		Node:   code.file,
	}

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.TypeSpec:
				out = append(out, TypeDeclaration{Node: value, Parent: parent})
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

func (function *Function) Params() []*ast.Field {
	return function.Node.Type.Params.List
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

	parent := NodeWithParent{
		Node: code.file,
	}

	var scope Scope

	mapTypeExpr, err := parser.ParseExpr(mapType)
	if err != nil {
		panic(errors.Wrapf(err, "invalid map type: %s", mapType))
	}

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			p := parent
			parent = NodeWithParent{
				Parent: &p,
				Node:   node,
			}

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

			return true
		},
	)

	return out
}

type SwitchStmt struct {
	Parent NodeWithParent
	Node   *ast.SwitchStmt
}

func (stmt *SwitchStmt) InsertAfter(node ast.Node) {
	insertAfter(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node}, node)
}

func (stmt *SwitchStmt) InsertBefore(node ast.Node) {
	insertBefore(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node}, node)
}

func (stmt *SwitchStmt) Remove() {
	remove(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node})
}

func (code *SourceFile) SwitchStatements() map[Scope][]SwitchStmt {
	out := make(map[Scope][]SwitchStmt)
	var parent NodeWithParent
	var scope Scope

	ast.Inspect(
		code.file,
		func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncDecl:
				scope = Scope{fun: value}

			case *ast.SwitchStmt:
				out[scope] = append(out[scope], SwitchStmt{Parent: parent, Node: value})
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

type IfStmt struct {
	Parent NodeWithParent
	Node   *ast.IfStmt
}

func (stmt *IfStmt) InsertAfter(node ast.Node) {
	insertAfter(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node}, node)
}

func (stmt *IfStmt) InsertBefore(node ast.Node) {
	insertBefore(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node}, node)
}

func (stmt *IfStmt) Remove() {
	remove(NodeWithParent{Parent: &stmt.Parent, Node: stmt.Node})
}

func (stmt *IfStmt) RemoveCondition() {
	blockStmt := stmt.Parent.FindUpstreamNode(&ast.BlockStmt{})

	if blockStmt == nil {
		return
	}

	blockStmt.Node.(*ast.BlockStmt).List = stmt.Node.Body.List
}

func (code *SourceFile) IfStatements() map[Scope][]IfStmt {
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
				out[scope] = append(out[scope], IfStmt{Parent: parent, Node: value})
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
			if ok && SourceCode(callExpr.Fun) == selector {
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
	Node   *ast.AssignStmt
}

func (assignment *Assignment) InsertAfter(node ast.Node) {
	insertAfter(NodeWithParent{Parent: &assignment.Parent, Node: assignment.Node}, node)
}

func (assignment *Assignment) InsertBefore(node ast.Node) {
	insertBefore(NodeWithParent{Parent: &assignment.Parent, Node: assignment.Node}, node)
}

func (assignment *Assignment) Remove() {
	if nodeWithParent := assignment.Parent.FindUpstreamNode(&ast.FuncDecl{}); nodeWithParent != nil {
		funDecl := nodeWithParent.Node.(*ast.FuncDecl)

		stmts := make([]ast.Stmt, 0)
		for _, stmt := range funDecl.Body.List {
			if reflect.DeepEqual(stmt, assignment.Node) {
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
	composite := assignment.Node.Rhs[0].(*ast.CompositeLit)
	return Struct{literal: composite}
}

func (assignment *Assignment) Replace(node ast.Stmt) {
	if funDecl := assignment.Parent.FindUpstreamNode(&ast.FuncDecl{}); funDecl != nil {
		block := funDecl.Node.(*ast.FuncDecl).Body

		for i := 0; i < len(block.List); i++ {
			assignStmt, ok := block.List[i].(*ast.AssignStmt)

			if ok &&
				reflect.DeepEqual(assignment.Node.Lhs, assignStmt.Lhs) &&
				reflect.DeepEqual(assignment.Node.Rhs, assignStmt.Rhs) {
				block.List[i] = node
			}
		}
	}
}

func (code *SourceFile) Assignments() map[Scope][]Assignment {
	assignments := make(map[Scope][]Assignment, 0)

	parent := NodeWithParent{
		Node: code.file,
	}

	var scope Scope

	ast.Inspect(code.file, func(node ast.Node) bool {
		p := parent
		parent = NodeWithParent{
			Parent: &p,
			Node:   node,
		}

		switch value := node.(type) {
		case *ast.FuncDecl:
			scope = Scope{fun: value}
		case *ast.BlockStmt:
			for _, statement := range value.List {
				stmt, ok := statement.(*ast.AssignStmt)
				if ok {
					assignments[scope] = append(assignments[scope], Assignment{
						Parent: parent,
						Node:   stmt,
					})
				}
			}
		}

		return true
	})

	return assignments
}

func (code *SourceFile) FindAssignments(target string) map[Scope][]Assignment {
	out := make(map[Scope][]Assignment, 0)

	for scope, assignments := range code.Assignments() {
		for _, assignment := range assignments {
			if NormalizeString(SourceCode(assignment.Node)) == NormalizeString(target) {
				out[scope] = append(out[scope], assignment)
				continue
			}

			for _, expr := range assignment.Node.Lhs {
				switch ident := expr.(type) {
				case *ast.SelectorExpr:
					if NormalizeString(SourceCode(ident)) == NormalizeString(target) {
						out[scope] = append(out[scope], assignment)
					}
				case *ast.Ident:
					if ident.Name == target {
						out[scope] = append(out[scope], assignment)
					}
				case *ast.IndexExpr:
					if NormalizeString(SourceCode(ident.X)) == NormalizeString(target) ||
						NormalizeString(SourceCode(ident)) == NormalizeString(target) {
						out[scope] = append(out[scope], assignment)
					}
				}
			}
		}
	}

	return out
}
