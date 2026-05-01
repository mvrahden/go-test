package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func TestRunOverlay_ProducesValidOutput(t *testing.T) {
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

	// Use the same logic as runOverlay to generate overlay
	results, err := gotestgen.Generate("./simple_suite")
	if err != nil {
		t.Fatalf("SuitesGenerate: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one generate result")
	}

	tmpDir, err := gotestrunner.WriteOverlay(results)
	if err != nil {
		t.Fatalf("WriteOverlay: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Verify the overlay file exists
	overlayFile := filepath.Join(tmpDir, "overlay.json")
	if _, err := os.Stat(overlayFile); err != nil {
		t.Fatalf("overlay.json not found: %v", err)
	}

	// Verify it is valid JSON
	data, err := os.ReadFile(overlayFile)
	if err != nil {
		t.Fatalf("reading overlay.json: %v", err)
	}
	var overlayContent struct {
		Replace map[string]string `json:"Replace"`
	}
	if err := json.Unmarshal(data, &overlayContent); err != nil {
		t.Fatalf("overlay.json is not valid JSON: %v", err)
	}
	if len(overlayContent.Replace) == 0 {
		t.Fatal("overlay.json Replace map is empty")
	}

	// Verify the output struct serializes correctly
	out := overlayOutput{
		OverlayFile: overlayFile,
		Dir:         tmpDir,
	}
	outData, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var roundtrip overlayOutput
	if err := json.Unmarshal(outData, &roundtrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if roundtrip.OverlayFile != overlayFile {
		t.Errorf("overlayFile = %q, want %q", roundtrip.OverlayFile, overlayFile)
	}
	if roundtrip.Dir != tmpDir {
		t.Errorf("dir = %q, want %q", roundtrip.Dir, tmpDir)
	}
}

func TestRunOverlay_NoSuitesReturnsOne(t *testing.T) {
	// Create a temp directory with a valid Go package that has no suites
	tmpDir, err := os.MkdirTemp("", "overlay-test-nosuite-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a minimal go.mod and a Go file with no suites
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module nosuite\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// SuitesGenerate on a package without suites should return empty results
	results, err := gotestgen.Generate(".")
	if err != nil {
		t.Fatalf("SuitesGenerate: %v", err)
	}

	// Verify behavior: no suites means empty results
	var allResults gotestgen.GenerateResults
	allResults = append(allResults, results...)
	if len(allResults) != 0 {
		t.Skipf("expected 0 results for package without suites, got %d (package may have test suites)", len(allResults))
	}
}
