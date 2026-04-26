package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(args []string) int {
	format, output, remaining := parseSpecFlags(args)

	ownArgs, goTestArgs := SplitArgs(remaining)
	DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")

	patterns := ExtractPackagePatterns(goTestArgs)

	var allResults gotestgen.GenerateResults
	for _, pattern := range patterns {
		results, _, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
	}

	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		defer os.RemoveAll(tmpDir)
	}

	overlayArgs := append([]string{"-overlay=" + filepath.Join(tmpDir, "overlay.json")}, goTestArgs...)

	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(overlayArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	var w io.Writer = os.Stdout
	if output != "" {
		f, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer f.Close()
		w = f
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree)
	}

	return code
}

func parseSpecFlags(args []string) (format, output string, remaining []string) {
	format = "terminal"
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--format="):
			format = strings.TrimPrefix(args[i], "--format=")
		case args[i] == "--output" && i+1 < len(args):
			output = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--output="):
			output = strings.TrimPrefix(args[i], "--output=")
		default:
			remaining = append(remaining, args[i])
		}
	}
	return
}
