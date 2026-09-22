package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/migrate"
)

// runMigrate converts testify suites in place. It exits 1 when a
// TODO(gotest-migrate) marker was left anywhere, so a migration that still
// needs a hand never reads as done; --dry-run prints the diff instead of
// writing and answers the same way.
func runMigrate(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	ownArgs, rest, err := SplitArgs(inv.Args, migrateAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	opts := migrate.Options{Out: os.Stdout}
	var patterns []string
	for _, arg := range rest {
		if len(arg) > 1 && arg[0] == '-' {
			fmt.Fprintf(os.Stderr, "FAIL: unknown flag %s — migrate accepts only --dry-run\n", arg)
			return 2
		}
		patterns = append(patterns, arg)
	}
	for _, arg := range ownArgs {
		if arg == "--dry-run" {
			opts.DryRun = true
		}
	}
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	results, err := migrate.MigratePackages(patterns, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 2
	}
	if len(results) == 0 {
		fmt.Println("No testify/suite patterns found.")
		return 0
	}
	verb := "Migrated"
	if opts.DryRun {
		verb = "Would migrate"
	}
	markers, refused := 0, 0
	var lines []string
	for _, r := range results {
		markers += r.Markers
		if r.Refusal != "" {
			refused++
			lines = append(lines, fmt.Sprintf("  %s: refused: %s", r.File, r.Refusal))
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s: %s → %s", r.File, r.OldName, r.NewName))
	}
	fmt.Printf("%s %d suites:\n", verb, len(results)-refused)
	for _, l := range lines {
		fmt.Println(l)
	}
	if markers == 0 {
		return 0
	}
	fmt.Printf("%d TODO(gotest-migrate) marker%s left; resolve them by hand\n", markers, plural(markers))
	return 1
}
