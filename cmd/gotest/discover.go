package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func runDiscover(args []string) int {
	patterns := ExtractPackagePatterns(args)

	allOutput := gotestgen.DiscoverOutput{}
	for _, pattern := range patterns {
		output, err := gotestgen.Discover(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allOutput.Packages = append(allOutput.Packages, output.Packages...)
	}

	data, err := json.Marshal(allOutput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}
