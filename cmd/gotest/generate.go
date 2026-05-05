package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func runGenerate(args []string) int {
	patterns := ExtractPackagePatterns(args)

	results, err := gotestgen.Generate(patterns, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	for _, r := range results {
		if len(r.PTest) > 0 {
			dst := filepath.Join(r.AbsPath, about.PSuite)
			if err := os.WriteFile(dst, r.PTest, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: writing %s: %s\n", dst, err)
				return 2
			}
			fmt.Println(dst)
		}
		if len(r.PXTest) > 0 {
			dst := filepath.Join(r.AbsPath, about.PXSuite)
			if err := os.WriteFile(dst, r.PXTest, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: writing %s: %s\n", dst, err)
				return 2
			}
			fmt.Println(dst)
		}
	}

	return 0
}
