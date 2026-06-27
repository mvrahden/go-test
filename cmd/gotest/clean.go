package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/about"
)

func runClean(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	patterns := ExtractPackagePatterns(inv.Args)

	var removed int
	for _, pattern := range patterns {
		dir := strings.TrimSuffix(pattern, "/...")
		if dir == "" || dir == "." {
			dir = "."
		}

		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") && name != "." || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if about.PSuiteRegex.MatchString(d.Name()) {
				if err := os.Remove(path); err != nil {
					fmt.Fprintf(os.Stderr, "warning: %s\n", err)
				} else {
					fmt.Println(path)
					removed++
				}
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: walking %s: %s\n", dir, err)
			return 2
		}
	}

	if removed > 0 {
		fmt.Fprintf(os.Stderr, "removed %d generated file(s)\n", removed)
	}
	return 0
}
