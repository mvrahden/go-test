package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/migrate"
)

// runMigrate converts testify suites in place and type-checks the result.
// It exits 0 only when every migrated package compiles and no
// TODO(gotest-migrate) marker is left anywhere; 1 when a marker is left,
// including one an earlier run placed, or an error has no line to carry
// one; 2 on a usage error or when a package does not compile before the
// migration, which then leaves it alone. A generated test file is skipped
// and named on stderr without changing the answer: its generator owns it,
// and a suite it uses is refused. --dry-run prints the diff instead of
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
	verb := "Migrated"
	if opts.DryRun {
		verb = "Would migrate"
	}
	markers, suites := 0, 0
	var lines, refused, leftover, uncompiled, unplaced, generated []string
	for i := range results {
		r := &results[i]
		markers += r.Markers
		switch {
		case len(r.Uncompiled) > 0:
			uncompiled = append(uncompiled, "  "+r.File+":")
			for _, e := range r.Uncompiled {
				uncompiled = append(uncompiled, "    "+e)
			}
		case len(r.Unplaced) > 0:
			for _, e := range r.Unplaced {
				unplaced = append(unplaced, "  "+r.File+": "+e)
			}
		case r.Generated:
			generated = append(generated, "  "+r.File)
		case r.Refusal != "":
			refused = append(refused, fmt.Sprintf("  %s: %s", r.File, r.Refusal))
		case r.RefsOnly:
			lines = append(lines, fmt.Sprintf("  %s: updated for suites other files declare", r.File))
		case r.OldName == "":
			// No suite left to convert; only an earlier run's markers.
			leftover = append(leftover, fmt.Sprintf("  %s: %d marker%s", r.File, r.Markers, plural(r.Markers)))
		default:
			suites++
			lines = append(lines, fmt.Sprintf("  %s: %s → %s", r.File, r.OldName, r.NewName))
		}
	}
	if len(lines) > 0 {
		fmt.Printf("%s %d suites:\n", verb, suites)
		printLines(lines)
	}
	if len(refused) > 0 {
		fmt.Println("Refused, left for hand migration:")
		printLines(refused)
	}
	if len(generated) > 0 {
		fmt.Fprintln(os.Stderr, "Skipped, generated (Code generated ... DO NOT EDIT.); migrate their generator:")
		for _, l := range generated {
			fmt.Fprintln(os.Stderr, l)
		}
	}
	if len(lines)+len(refused)+len(leftover)+len(uncompiled)+len(unplaced)+len(generated) == 0 {
		fmt.Println("No testify/suite patterns found.")
	}
	if len(leftover) > 0 {
		fmt.Println("Markers an earlier run left, still to resolve:")
		printLines(leftover)
	}
	if len(uncompiled) > 0 {
		fmt.Println("Not migrated: these packages do not compile as they are, so nothing about a migration could be proven:")
		printLines(uncompiled)
	}
	if len(unplaced) > 0 {
		fmt.Println("The migrated code does not compile, with errors no marker could carry:")
		printLines(unplaced)
	}
	if markers > 0 {
		fmt.Printf("%d TODO(gotest-migrate) marker%s left; resolve them by hand\n", markers, plural(markers))
	}
	switch {
	case len(uncompiled) > 0:
		return 2
	case markers > 0 || len(unplaced) > 0:
		return 1
	}
	return 0
}

func printLines(lines []string) {
	for _, l := range lines {
		fmt.Println(l)
	}
}
