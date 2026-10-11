package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/migrate"
)

// runMigrate converts testify suites in place. It exits 1 when a
// TODO(gotest-migrate) marker is left anywhere, including one an earlier run
// placed, so a migration that still needs a hand never reads as done;
// --dry-run prints the diff instead of writing and answers the same way.
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
	verb := "Migrated"
	if opts.DryRun {
		verb = "Would migrate"
	}
	markers, suites := 0, 0
	var lines, leftover []string
	for _, r := range results {
		markers += r.Markers
		switch {
		case r.Refusal != "":
			lines = append(lines, fmt.Sprintf("  %s: refused: %s", r.File, r.Refusal))
		case r.RefsOnly:
			lines = append(lines, fmt.Sprintf("  %s: references to renamed suites updated", r.File))
		case r.OldName == "":
			// No suite left to convert; only an earlier run's markers.
			leftover = append(leftover, fmt.Sprintf("  %s: %d marker%s", r.File, r.Markers, plural(r.Markers)))
		default:
			suites++
			lines = append(lines, fmt.Sprintf("  %s: %s → %s", r.File, r.OldName, r.NewName))
		}
	}
	if len(lines) == 0 {
		fmt.Println("No testify/suite patterns found.")
	} else {
		fmt.Printf("%s %d suites:\n", verb, suites)
		for _, l := range lines {
			fmt.Println(l)
		}
	}
	if len(leftover) > 0 {
		fmt.Println("Left by an earlier run:")
		for _, l := range leftover {
			fmt.Println(l)
		}
	}
	if markers == 0 {
		return 0
	}
	fmt.Printf("%d TODO(gotest-migrate) marker%s left; resolve them by hand\n", markers, plural(markers))
	return 1
}
