package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/scaffold"
)

func runScaffold(args []string) int {
	var target string
	for _, arg := range args {
		if !isFlag(arg) {
			target = arg
			break
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "usage: gotest scaffold ./pkg/path.TypeName or ./path/file.go")
		return 1
	}

	if strings.HasSuffix(target, ".go") {
		return runScaffoldFile(target)
	}
	return runScaffoldType(target)
}

func runScaffoldFile(target string) int {
	infos, err := scaffold.IntrospectFile(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	for _, info := range infos {
		filename := scaffold.ToSnakeCase(info.Name) + "_suite_test.go"
		outPath := filepath.Join(info.PkgDir, filename)

		if _, err := os.Stat(outPath); err == nil {
			fmt.Fprintf(os.Stderr, "scaffold: %s already exists\n", outPath)
			return 1
		}

		var out []byte
		switch {
		case info.IsFuncBased:
			out, err = scaffold.GenerateFuncScaffold(info)
		case info.IsInterface:
			out, err = scaffold.GenerateContractScaffold(info)
		default:
			out, err = scaffold.GenerateScaffold(info)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
			return 1
		}

		if err := os.WriteFile(outPath, out, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "scaffold: failed to write %s: %v\n", outPath, err)
			return 1
		}
		writeGenerated(outPath)
	}
	return 0
}

func runScaffoldType(target string) int {
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

	filename := scaffold.ToSnakeCase(typeName) + "_suite_test.go"
	outPath := filepath.Join(info.PkgDir, filename)

	if _, err := os.Stat(outPath); err == nil {
		fmt.Fprintf(os.Stderr, "scaffold: %s already exists\n", outPath)
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

	if err := os.WriteFile(outPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: failed to write %s: %v\n", outPath, err)
		return 1
	}
	writeGenerated(outPath)
	return 0
}

func writeGenerated(outPath string) {
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		if rel, err := filepath.Rel(cwd, outPath); err == nil {
			fmt.Printf("Generated: %s\n", rel)
			return
		}
	}
	fmt.Printf("Generated: %s\n", outPath)
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
