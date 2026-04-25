package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/scaffold"
)

func runScaffold(args []string) int {
	// Find first non-flag argument as target
	var target string
	for _, arg := range args {
		if !isFlag(arg) {
			target = arg
			break
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: gotest scaffold ./pkg/path.TypeName")
		return 1
	}

	pkgPattern, typeName, err := scaffold.ParseTarget(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	info, err := scaffold.IntrospectType(pkgPattern, typeName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	var out []byte
	if info.IsInterface {
		out, err = scaffold.GenerateContractScaffold(info)
	} else {
		out, err = scaffold.GenerateScaffold(info)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	filename := scaffold.ToSnakeCase(typeName) + "_suite_test.go"
	outPath := filepath.Join(info.PkgDir, filename)

	if err := os.WriteFile(outPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: failed to write %s: %v\n", outPath, err)
		return 1
	}

	// Print relative path if possible, falling back to absolute
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		if rel, err := filepath.Rel(cwd, outPath); err == nil {
			fmt.Printf("Generated: %s\n", rel)
			return 0
		}
	}
	fmt.Printf("Generated: %s\n", outPath)

	return 0
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
