package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func TestSpecFlagParsing(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantFmt   string
		wantOut   string
		wantInput string
		wantColor bool
		wantGoLen int
	}{
		{
			name:      "no flags",
			args:      []string{"./..."},
			wantFmt:   "terminal",
			wantInput: "",
			wantColor: false,
			wantGoLen: 1,
		},
		{
			name:      "input with equals",
			args:      []string{"--input=events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			name:      "input with space",
			args:      []string{"--input", "events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			name:      "input stdin dash",
			args:      []string{"--input=-"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			name:      "input with format",
			args:      []string{"--format=md", "--input=data.json"},
			wantFmt:   "md",
			wantInput: "data.json",
			wantColor: false,
			wantGoLen: 0,
		},
		{
			name:      "input with output and no-color",
			args:      []string{"--input=-", "--output=out.txt", "--no-color"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantOut:   "out.txt",
			wantColor: true,
			wantGoLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownArgs, goTestArgs, err := SplitArgs(tt.args, specAllowed)
			if err != nil {
				t.Fatalf("SplitArgs: %v", err)
			}

			format := extractStringFlag(ownArgs, "--format", "terminal")
			output := extractStringFlag(ownArgs, "--output", "")
			input := extractStringFlag(ownArgs, "--input", "")
			noColor := hasFlag(ownArgs, "--no-color")

			if format != tt.wantFmt {
				t.Errorf("format = %q, want %q", format, tt.wantFmt)
			}
			if output != tt.wantOut {
				t.Errorf("output = %q, want %q", output, tt.wantOut)
			}
			if input != tt.wantInput {
				t.Errorf("input = %q, want %q", input, tt.wantInput)
			}
			if noColor != tt.wantColor {
				t.Errorf("noColor = %v, want %v", noColor, tt.wantColor)
			}
			if len(goTestArgs) != tt.wantGoLen {
				t.Errorf("goTestArgs = %v (len %d), want len %d", goTestArgs, len(goTestArgs), tt.wantGoLen)
			}
		})
	}
}

func TestRunSpec_InputStdin(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	if _, err := os.Stat(filepath.Join(examplesDir, "go.mod")); err != nil {
		t.Skipf("examples directory not found: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absExamples, err := filepath.Abs(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(absExamples); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	loaded, err := gotestgen.LoadPackages([]string{"./cart"}, nil)
	if err != nil {
		t.Fatalf("LoadPackages: %v", err)
	}
	results, _, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		t.Fatalf("GenerateFromLoaded: %v", err)
	}

	tmpDir, err := gotestrunner.WriteOverlay(results)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	overlayArgs := []string{"-overlay=" + filepath.Join(tmpDir, "overlay.json"), "./cart"}
	jsonData, _, err := gotestrunner.StdlibRunTestsJSON(context.Background(), overlayArgs)
	if err != nil {
		t.Fatalf("StdlibRunTestsJSON: %v", err)
	}

	// Now parse and render the JSON output as the --input path would
	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}

	tree := gotestspec.BuildTree(events)

	var buf bytes.Buffer
	gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("ShoppingCart")) {
		t.Errorf("expected output to contain \"ShoppingCart\", got:\n%s", output)
	}
}
