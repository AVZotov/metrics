// Package buildinfo prints build metadata injected via -ldflags -X.
package buildinfo

import "fmt"

// Print writes version, date, and commit to stdout, substituting "N/A" for
// any value left empty (i.e. not set via -ldflags -X at build time).
func Print(version, date, commit string) {
	const def = "N/A"
	fmt.Printf("Build Version: %s\n", orDefault(version, def))
	fmt.Printf("Build Date: %s\n", orDefault(date, def))
	fmt.Printf("Build Commit: %s\n", orDefault(commit, def))
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
