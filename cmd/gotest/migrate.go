package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/migrate"
)

func runMigrate(args []string) int {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	results, err := migrate.MigratePackages(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Println("No testify/suite patterns found.")
		return 0
	}
	fmt.Printf("Migrated %d suites:\n", len(results))
	for _, r := range results {
		fmt.Printf("  %s: %s → %s\n", r.File, r.OldName, r.NewName)
	}
	return 0
}
