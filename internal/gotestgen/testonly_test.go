package gotestgen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func TestIsTestOnly(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	absExamples, err := filepath.Abs(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(absExamples); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	tests := []struct {
		pattern  string
		expected bool
	}{
		{"./cart", false},
		{"./auth", false},
		{"./search", false},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			results, _, err := gotestgen.LoadPackagesForDiscovery([]string{tc.pattern}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 {
				t.Fatal("no packages found")
			}
			got := results[0].IsTestOnly()
			if got != tc.expected {
				t.Errorf("IsTestOnly() = %v, want %v", got, tc.expected)
			}
		})
	}
}
