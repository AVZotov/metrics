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
	analyzers = append(analyzers, printf.Analyzer)
	analyzers = append(analyzers, shadow.Analyzer)
	analyzers = append(analyzers, structtag.Analyzer)
	analyzers = append(analyzers, unreachable.Analyzer)
	analyzers = append(analyzers, nilfunc.Analyzer)
	analyzers = append(analyzers, atomic.Analyzer)
	analyzers = append(analyzers, bools.Analyzer)
	analyzers = append(analyzers, errorsas.Analyzer)
	analyzers = append(analyzers, assign.Analyzer)
	analyzers = append(analyzers, composite.Analyzer)
	
	for _, a := range staticcheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	for _, a := range simple.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	for _, a := range stylecheck.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	for _, a := range quickfix.Analyzers {
		analyzers = append(analyzers, a.Analyzer)
	}
	analyzers = append(analyzers, unused.Analyzer.Analyzer)
	
	analyzers = append(analyzers, errcheck.Analyzer)
	
	analyzers = append(analyzers, bodyclose.Analyzer)
	
	analyzers = append(analyzers, osexitanalyzer.Analyzer)
	
	multichecker.Main(analyzers...)
	
}
