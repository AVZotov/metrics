// Package osexitanalyzer implements a custom analyzer that flags direct
// os.Exit calls inside the main function of package main. It skips main()
// in non-main packages, skips methods (a func named "main" with a
// receiver), and skips synthesized "<pkg>.test" packages so it doesn't
// trip on the test binary's generated entry point. See the Analyzer var's
// doc comment for the same rule in godoc form.
package osexitanalyzer

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports direct calls to os.Exit inside the main function of
// package main, skipping main() in non-main packages, methods named
// "main", and synthesized "<pkg>.test" packages.
var Analyzer = &analysis.Analyzer{
	Name: "os_exit_check",
	Doc:  "os exit analyzer\n\ncheck os.exit call in main func of main package",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	if strings.HasSuffix(pass.Pkg.Path(), ".test") {
		return nil, nil
	}

	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok {
				if funcDecl.Name != nil && funcDecl.Name.Name == "main" && funcDecl.Recv == nil {
					ast.Inspect(
						funcDecl.Body, func(n ast.Node) bool {
							call, ok := n.(*ast.CallExpr)
							if !ok {
								return true
							}
							if isOsExitCall(call) {
								pass.Reportf(
									call.Pos(),
									"direct call to os.Exit is not allowed in main function of package main",
								)
							}

							return true
						},
					)
				}
			}
		}
	}

	return nil, nil
}

func isOsExitCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "os" {
		return false
	}

	return sel.Sel.Name == "Exit"
}
