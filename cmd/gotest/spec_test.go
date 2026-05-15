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

func TestParseSpecFlags_Input(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantFmt   string
		wantOut   string
		wantInput string
		wantColor bool
		wantRest  []string
	}{
		{
			name:      "no flags",
			args:      []string{"./..."},
			wantFmt:   "terminal",
			wantInput: "",
			wantColor: false,
			wantRest:  []string{"./..."},
		},
		{
			name:      "input with equals",
			args:      []string{"--input=events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantRest:  nil,
		},
		{
			name:      "input with space",
			args:      []string{"--input", "events.json"},
			wantFmt:   "terminal",
			wantInput: "events.json",
			wantColor: false,
			wantRest:  nil,
		},
		{
			name:      "input stdin dash",
			args:      []string{"--input=-"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantColor: false,
			wantRest:  nil,
		},
		{
			name:      "input with format",
			args:      []string{"--format=md", "--input=data.json"},
			wantFmt:   "md",
			wantInput: "data.json",
			wantColor: false,
			wantRest:  nil,
		},
		{
			name:      "input with output and no-color",
			args:      []string{"--input=-", "--output=out.txt", "--no-color"},
			wantFmt:   "terminal",
			wantInput: "-",
			wantOut:   "out.txt",
			wantColor: true,
			wantRest:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, output, input, noColor, remaining := parseSpecFlags(tt.args)
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
			if len(remaining) != len(tt.wantRest) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRest)
			} else {
				for i := range remaining {
					if remaining[i] != tt.wantRest[i] {
						t.Errorf("remaining[%d] = %q, want %q", i, remaining[i], tt.wantRest[i])
					}
				}
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

	loaded, err := gotestgen.LoadPackages([]string{"./simple_suite"}, nil)
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

	overlayArgs := []string{"-overlay=" + filepath.Join(tmpDir, "overlay.json"), "./simple_suite"}
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
	if !bytes.Contains([]byte(output), []byte("Simple")) {
		t.Errorf("expected output to contain \"Simple\", got:\n%s", output)
	}
}
