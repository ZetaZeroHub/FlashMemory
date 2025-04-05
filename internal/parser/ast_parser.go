package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"strings"
)

// --- Implementation: Go AST Parser (for Go code) ---

// Deprecated: GoASTParser is deprecated and will be removed in a future version.
// Please use the new parser implementation with Tree-sitter support instead.
type GoASTParser struct{}

func (p *GoASTParser) ParseFile(path string) ([]FunctionInfo, error) {
	src, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	fileAST, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	pkgName := fileAST.Name.Name
	funcs := []FunctionInfo{}

	// Collect imports from AST
	importList := []string{}
	for _, imp := range fileAST.Imports {
		impPath := strings.Trim(imp.Path.Value, "\"") // remove quotes
		importList = append(importList, impPath)
	}

	// Traverse AST for function declarations
	for _, decl := range fileAST.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fn := FunctionInfo{
			Name:    funcDecl.Name.Name,
			File:    path,
			Package: pkgName,
			Imports: importList,
			Calls:   []string{},
		}
		// If method (has receiver)
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			// e.g. "*User" or "User" type name
			var recvType string
			switch t := funcDecl.Recv.List[0].Type.(type) {
			case *ast.StarExpr:
				// Pointer receiver (e.g. *StructWithMethods)
				if ident, ok := t.X.(*ast.Ident); ok {
					recvType = "*" + ident.Name
				}
			case *ast.Ident:
				// Value receiver (e.g. StructWithMethods)
				recvType = t.Name
			default:
				// Fallback for other cases
				recvType = fmt.Sprintf("%v", funcDecl.Recv.List[0].Type)
			}
			fn.Receiver = recvType
			// Prepend receiver type to function name for uniqueness (like User.Save)
			fn.Name = recvType + "." + fn.Name
		}
		// Parameter list (names and types)
		params := []string{}
		for _, field := range funcDecl.Type.Params.List {
			for _, name := range field.Names {
				paramType := fmt.Sprintf("%s", field.Type)
				params = append(params, fmt.Sprintf("%s %s", name.Name, paramType))
			}
			if len(field.Names) == 0 {
				// anonymous parameter (like ... or unused)
				paramType := fmt.Sprintf("%s", field.Type)
				params = append(params, paramType)
			}
		}
		fn.Parameters = params
		// Count lines of function body (simple way: end line - start line)
		if funcDecl.Body != nil {
			start := fset.Position(funcDecl.Body.Lbrace).Line
			end := fset.Position(funcDecl.Body.Rbrace).Line
			fn.Lines = end - start + 1
		}
		// Find calls inside the function body (traverse AST nodes)
		if funcDecl.Body != nil {
			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					// Get function name being called
					var callName string
					switch f := call.Fun.(type) {
					case *ast.Ident:
						callName = f.Name
					case *ast.SelectorExpr:
						// e.g. pkg.Func or obj.Method
						if pkgIdent, ok := f.X.(*ast.Ident); ok {
							callName = pkgIdent.Name + "." + f.Sel.Name
						} else {
							callName = f.Sel.Name
						}
					}
					if callName != "" {
						fn.Calls = append(fn.Calls, callName)
					}
				}
				return true
			})
		}
		funcs = append(funcs, fn)
	}
	return funcs, nil
}
