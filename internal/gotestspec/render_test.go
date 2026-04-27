package gotestspec

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func TestRenderTerminal_SuiteHierarchy(t *testing.T) {
	packages := []*Package{{
		Path: "example.com/pkg",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "UserService",
			Children: []*Node{{
				Kind:    KindMethod,
				Display: "Create",
				Children: []*Node{{
					Kind:     KindBlock,
					Display:  "returns ok",
					Status:   StatusPass,
					Duration: 8 * time.Millisecond,
				}},
			}},
		}},
	}}

	var buf bytes.Buffer
	RenderTerminal(&buf, packages)
	out := stripANSI(buf.String())

	for _, want := range []string{"UserService", "Create", "✓ returns ok", "(8ms)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderTerminal_FailedLeaf(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:    KindTest,
			Display: "Broken",
			Children: []*Node{{
				Kind:     KindBlock,
				Display:  "explodes",
				Status:   StatusFail,
				Duration: 2 * time.Millisecond,
				Output:   []string{"    expected 1, got 2\n"},
			}},
		}},
	}}

	var buf bytes.Buffer
	RenderTerminal(&buf, packages)
	out := stripANSI(buf.String())

	if !strings.Contains(out, "✗ explodes") {
		t.Errorf("output missing failure icon:\n%s", out)
	}
	if !strings.Contains(out, "expected 1, got 2") {
		t.Errorf("output missing error detail:\n%s", out)
	}
}

func TestRenderTerminal_MultiPackage(t *testing.T) {
	packages := []*Package{
		{Path: "a/pkg", Nodes: []*Node{{Kind: KindTest, Display: "Foo", Status: StatusPass, Duration: time.Millisecond}}},
		{Path: "b/pkg", Nodes: []*Node{{Kind: KindTest, Display: "Bar", Status: StatusPass, Duration: time.Millisecond}}},
	}

	var buf bytes.Buffer
	RenderTerminal(&buf, packages)
	out := stripANSI(buf.String())

	if !strings.Contains(out, "=== a/pkg ===") {
		t.Errorf("output missing first package header:\n%s", out)
	}
	if !strings.Contains(out, "=== b/pkg ===") {
		t.Errorf("output missing second package header:\n%s", out)
	}
}

func TestRenderTerminal_SummaryLine(t *testing.T) {
	tests := []struct {
		name  string
		stats Stats
		want  string
	}{
		{
			"suites only",
			Stats{Suites: 2, Behaviors: 5, Passed: 5},
			"2 suites, 5 behaviors: 5 passed",
		},
		{
			"stdlib only",
			Stats{Tests: 3, Passed: 2, Failed: 1},
			"3 stdlib tests: 2 passed, 1 failed",
		},
		{
			"mixed",
			Stats{Suites: 1, Behaviors: 2, Tests: 1, Passed: 2, Skipped: 1},
			"1 suites, 2 behaviors, 1 stdlib tests: 2 passed, 1 skipped",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderSummary(&buf, tt.stats, ansiColors)
			got := stripANSI(buf.String())
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderMarkdown_SuiteHierarchy(t *testing.T) {
	packages := []*Package{{
		Path: "example.com/pkg",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "UserService",
			Children: []*Node{{
				Kind:    KindMethod,
				Display: "Create",
				Children: []*Node{{
					Kind:     KindBlock,
					Display:  "returns ok",
					Status:   StatusPass,
					Duration: 8 * time.Millisecond,
				}},
			}},
		}},
	}}

	var buf bytes.Buffer
	RenderMarkdown(&buf, packages)
	out := buf.String()

	for _, want := range []string{
		"# Behavior Specification",
		"## UserService",
		"### Create",
		"| returns ok | PASS | 8ms |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMarkdown_SkippedSuite(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:     KindSuite,
			Display:  "Broken",
			Excluded: true,
			Status:   StatusSkip,
		}},
	}}

	var buf bytes.Buffer
	RenderMarkdown(&buf, packages)
	out := buf.String()

	if !strings.Contains(out, "Broken — SKIPPED") {
		t.Errorf("output missing SKIPPED label:\n%s", out)
	}
}
