package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(args []string) int {
	format, output, input, noColor, remaining := parseSpecFlags(args)

	if input != "" {
		return runSpecFromInput(input, format, output, noColor)
	}

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

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		var serr error
		setupProc, serr = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures)
		if serr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", serr)
			return 2
		}
		defer setupProc.Teardown()
		extraEnv["GOTEST_SHARED_STATE_FILE"] = setupProc.StateFile()
	}

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

func runSpecFromInput(input, format, output string, noColor bool) int {
	var r io.Reader
	if input == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: opening input file: %s\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}

	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	var w io.Writer = os.Stdout
	if output != "" {
		of, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer of.Close()
		w = of
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

	return 0
}

func parseSpecFlags(args []string) (format, output, input string, noColor bool, remaining []string) {
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
		case args[i] == "--input" && i+1 < len(args):
			input = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--input="):
			input = strings.TrimPrefix(args[i], "--input=")
		case args[i] == "--no-color":
			noColor = true
		default:
			remaining = append(remaining, args[i])
		}
	}
	return
}
