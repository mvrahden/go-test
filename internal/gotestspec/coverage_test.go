package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseCoverageReader(t *testing.T) {
	profile := `mode: atomic
github.com/user/repo/pkg/foo/foo.go:10.20,12.2 1 5
github.com/user/repo/pkg/foo/foo.go:14.30,16.2 1 0
github.com/user/repo/pkg/bar/bar.go:5.10,8.2 3 1
github.com/user/repo/pkg/bar/bar.go:10.10,12.2 1 0
`
	report, err := parseCoverageReader(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(report.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(report.Packages))
	}

	// Sorted by path
	bar := report.Packages[0]
	foo := report.Packages[1]

	if bar.Path != "github.com/user/repo/pkg/bar" {
		t.Errorf("bar path = %q", bar.Path)
	}
	if bar.Covered != 3 || bar.Total != 4 {
		t.Errorf("bar covered/total = %d/%d, want 3/4", bar.Covered, bar.Total)
	}
	if math.Abs(bar.Percentage-75.0) > 0.1 {
		t.Errorf("bar percentage = %.1f, want 75.0", bar.Percentage)
	}

	if foo.Path != "github.com/user/repo/pkg/foo" {
		t.Errorf("foo path = %q", foo.Path)
	}
	if foo.Covered != 1 || foo.Total != 2 {
		t.Errorf("foo covered/total = %d/%d, want 1/2", foo.Covered, foo.Total)
	}
	if math.Abs(foo.Percentage-50.0) > 0.1 {
		t.Errorf("foo percentage = %.1f, want 50.0", foo.Percentage)
	}

	// Total: 4 covered / 6 total = 66.7%
	if math.Abs(report.Total-66.7) > 0.1 {
		t.Errorf("total = %.1f, want 66.7", report.Total)
	}
}

func TestParseCoverageReader_Empty(t *testing.T) {
	report, err := parseCoverageReader(strings.NewReader("mode: set\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Total != 0 {
		t.Errorf("total = %.1f, want 0", report.Total)
	}
	if len(report.Packages) != 0 {
		t.Errorf("expected 0 packages, got %d", len(report.Packages))
	}
}

func TestParseCoverageLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		file  string
		stmts int
		count int
		err   bool
	}{
		{
			name:  "standard line",
			input: "github.com/user/repo/foo.go:10.20,12.2 1 5",
			file:  "github.com/user/repo/foo.go",
			stmts: 1,
			count: 5,
		},
		{
			name:  "uncovered",
			input: "pkg/bar.go:5.1,8.3 3 0",
			file:  "pkg/bar.go",
			stmts: 3,
			count: 0,
		},
		{
			name:  "empty line",
			input: "",
			err:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, stmts, count, err := parseCoverageLine(tt.input)
			if tt.err {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if file != tt.file {
				t.Errorf("file = %q, want %q", file, tt.file)
			}
			if stmts != tt.stmts {
				t.Errorf("stmts = %d, want %d", stmts, tt.stmts)
			}
			if count != tt.count {
				t.Errorf("count = %d, want %d", count, tt.count)
			}
		})
	}
}

func TestRenderSummary_WithCoverage(t *testing.T) {
	packages := []*Package{{
		Path:     "p",
		Duration: time.Second,
		Nodes: []*Node{
			{Kind: KindTest, Display: "A", Status: StatusPass},
		},
	}}

	report := &CoverageReport{
		Total: 82.4,
		Packages: []PackageCoverage{
			{Path: "p", Percentage: 82.4, Covered: 41, Total: 50},
		},
	}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor(), WithCoverage(report))
	out := buf.String()

	if !strings.Contains(out, "Coverage: 82.4%") {
		t.Errorf("expected coverage line, got:\n%s", out)
	}
}

func TestRenderMarkdownSummary_WithCoverage(t *testing.T) {
	packages := []*Package{{
		Path:     "p",
		Duration: time.Second,
		Nodes: []*Node{
			{Kind: KindTest, Display: "A", Status: StatusPass},
		},
	}}

	report := &CoverageReport{
		Total: 75.0,
		Packages: []PackageCoverage{
			{Path: "pkg/foo", Percentage: 90.0, Covered: 9, Total: 10},
			{Path: "pkg/bar", Percentage: 60.0, Covered: 6, Total: 10},
		},
	}

	var buf bytes.Buffer
	RenderMarkdownSummary(&buf, packages, WithCoverage(report))
	out := buf.String()

	if !strings.Contains(out, "### Coverage: 75.0%") {
		t.Errorf("expected coverage heading, got:\n%s", out)
	}
	if !strings.Contains(out, "| `pkg/foo` | 90.0% |") {
		t.Errorf("expected foo row, got:\n%s", out)
	}
	if !strings.Contains(out, "| `pkg/bar` | 60.0% |") {
		t.Errorf("expected bar row, got:\n%s", out)
	}
}
