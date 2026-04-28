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
		fmt.Fprintln(os.Stderr, "usage: gotest scaffold <./pkg/path.TypeName | ./pkg/path/file.go>")
		return 1
	}

	if strings.HasSuffix(target, ".go") {
		return runScaffoldFile(target)
	}
	return runScaffoldType(target)
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
	return writeScaffoldFile(info.PkgDir, filename, out)
}

func runScaffoldFile(target string) int {
	dir := filepath.Dir(target)
	filename := filepath.Base(target)
	pkgPattern := "./" + filepath.ToSlash(dir)

	info, err := scaffold.IntrospectFile(pkgPattern, filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	out, err := scaffold.GenerateFileScaffold(info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: %v\n", err)
		return 1
	}

	outFilename := scaffold.ToSnakeCase(strings.TrimSuffix(filename, ".go")) + "_suite_test.go"
	return writeScaffoldFile(info.PkgDir, outFilename, out)
}

func writeScaffoldFile(pkgDir, filename string, content []byte) int {
	outPath := filepath.Join(pkgDir, filename)

	if err := os.WriteFile(outPath, content, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "scaffold: failed to write %s: %v\n", outPath, err)
		return 1
	}
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
