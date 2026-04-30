package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(args []string) int {
	format, output, noColor, remaining := parseSpecFlags(args)

	ownArgs, goTestArgs := SplitArgs(remaining)
	DEBUG = slices.Contains(ownArgs, "--debug")
	CI = slices.Contains(ownArgs, "--ci")
	UPDATE_SNAPSHOTS = slices.Contains(ownArgs, "--update-snapshots")

	patterns := ExtractPackagePatterns(goTestArgs)

	if CI {
		violations, err := RunFocusGuard(patterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	overlay, cleanup, err := generateOverlay(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	overlayArgs := append([]string{overlay.overlayFlag}, goTestArgs...)
	extraEnv := buildExtraEnv()

	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(overlayArgs, extraEnv)
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

	var renderOpts []gotestspec.RenderOption
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree, renderOpts...)
	}

	return code
}

func parseSpecFlags(args []string) (format, output string, noColor bool, remaining []string) {
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
		case args[i] == "--no-color":
			noColor = true
		default:
			remaining = append(remaining, args[i])
		}
	}
	return
}
