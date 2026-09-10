// Package main is the staticlint multichecker: a single binary bundling
// several static analyzers behind the standard golang.org/x/tools
// multichecker runner. Build it and run it like any other analysis tool:
//
//	go build -o staticlint ./cmd/staticlint
//	./staticlint ./...
//
// It combines four groups of analyzers: the standard
// golang.org/x/tools/go/analysis/passes analyzers (added individually
// below); all of staticcheck's SA-class analyzers (bug/correctness
// checks); staticcheck's other analyzer classes — simple/S (simplification
// suggestions), stylecheck/ST (style conventions), quickfix/QF (suggested
// quick fixes) and unused/U (dead code detection); and two public
// third-party analyzers, errcheck (unchecked errors) and bodyclose
// (unclosed HTTP response bodies). On top of all that it adds the
// project's own osexitanalyzer, which forbids direct os.Exit calls in
// main's main function.
package main

import (
	osexitanalyzer "github.com/AVZotov/metrics/cmd/staticlint/osexit_analyzer"
	"github.com/kisielk/errcheck/errcheck"
	"github.com/timakin/bodyclose/passes/bodyclose"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"honnef.co/go/tools/quickfix"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
	"honnef.co/go/tools/unused"
)

func main() {
	var analyzers []*analysis.Analyzer
	// printf.Analyzer: checks Printf-style format string args match arguments
	analyzers = append(analyzers, printf.Analyzer)
	// shadow.Analyzer: checks for shadowed variables
	analyzers = append(analyzers, shadow.Analyzer)
	// structtag.Analyzer: checks struct field tags conform to reflect.StructTag conventions
	analyzers = append(analyzers, structtag.Analyzer)
	// unreachable.Analyzer: checks for unreachable code
	analyzers = append(analyzers, unreachable.Analyzer)
	// nilfunc.Analyzer: checks for useless comparisons of functions to nil
	analyzers = append(analyzers, nilfunc.Analyzer)
	// atomic.Analyzer: checks for common mistakes using the sync/atomic package
	analyzers = append(analyzers, atomic.Analyzer)
	// bools.Analyzer: checks for common mistakes involving boolean operators
	analyzers = append(analyzers, bools.Analyzer)
	// errorsas.Analyzer: checks that errors.As target is a pointer to a type implementing error
	analyzers = append(analyzers, errorsas.Analyzer)
	// assign.Analyzer: checks for useless assignments (x = x)
	analyzers = append(analyzers, assign.Analyzer)
	// composite.Analyzer: checks for unkeyed composite literals
	analyzers = append(analyzers, composite.Analyzer)

	// SA class: staticcheck's own analyzers, covering actual bugs and correctness issues
	for _, a := range staticcheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	// S class: simple.Analyzers suggest code simplifications
	for _, a := range simple.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	// ST class: stylecheck.Analyzers enforce style conventions
	for _, a := range stylecheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	// QF class: quickfix.Analyzers flag places with a suggested quick fix
	for _, a := range quickfix.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	// U class: unused.Analyzer finds dead code (unused identifiers)
	analyzers = append(analyzers, unused.Analyzer.Analyzer)

	// errcheck.Analyzer: checks for unchecked returned errors
	analyzers = append(analyzers, errcheck.Analyzer)

	// bodyclose.Analyzer: checks that HTTP response bodies are closed
	analyzers = append(analyzers, bodyclose.Analyzer)

	// osexitanalyzer.Analyzer: project-specific check, forbids direct os.Exit calls in main's main function
	analyzers = append(analyzers, osexitanalyzer.Analyzer)

	multichecker.Main(analyzers...)

}
