package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderSummary_AllPass(t *testing.T) {
	packages := []*Package{{
		Path:     "example.com/pkg",
		Duration: 2300 * time.Millisecond,
		Nodes: []*Node{
			{Kind: KindTest, Display: "Foo", Status: StatusPass, Duration: time.Second},
			{Kind: KindTest, Display: "Bar", Status: StatusPass, Duration: time.Second},
		},
	}}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "2 tests passed") {
		t.Errorf("expected success line, got:\n%s", out)
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("expected no FAIL in output:\n%s", out)
	}
}

func TestRenderSummary_WithFailures(t *testing.T) {
	packages := []*Package{{
		Path: "example.com/pkg/foo",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "UserService",
			Children: []*Node{
				{
					Kind:    KindMethod,
					Display: "Create",
					Children: []*Node{
						{
							Kind:     KindBlock,
							Display:  "returns ok",
							Status:   StatusPass,
							Duration: 5 * time.Millisecond,
						},
						{
							Kind:     KindBlock,
							Display:  "rejects empty name",
							Status:   StatusFail,
							Duration: 12 * time.Millisecond,
							Output:   []string{"    user_test.go:42: expected error, got nil\n"},
						},
					},
				},
				{
					Kind:     KindMethod,
					Display:  "Delete",
					Status:   StatusPass,
					Duration: 3 * time.Millisecond,
				},
			},
		}},
	}}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "1 of 3 tests failed") {
		t.Errorf("expected failure count, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL") {
		t.Errorf("expected FAIL label, got:\n%s", out)
	}
	if !strings.Contains(out, "UserService / Create / rejects empty name") {
		t.Errorf("expected failure path, got:\n%s", out)
	}
	if !strings.Contains(out, "expected error, got nil") {
		t.Errorf("expected error message, got:\n%s", out)
	}
	if strings.Contains(out, "returns ok") {
		t.Errorf("passing tests should not appear in summary:\n%s", out)
	}
}

func TestRenderSummary_FiltersNoise(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:     KindTest,
			Display:  "Broken",
			Status:   StatusFail,
			Duration: time.Millisecond,
			Output: []string{
				"=== RUN   TestBroken\n",
				"--- FAIL: TestBroken (0.00s)\n",
				"    broken_test.go:10: assertion failed\n",
			},
		}},
	}}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "assertion failed") {
		t.Errorf("expected assertion message, got:\n%s", out)
	}
	if strings.Contains(out, "=== RUN") {
		t.Errorf("should filter === RUN noise:\n%s", out)
	}
	if strings.Contains(out, "--- FAIL") {
		t.Errorf("should filter --- FAIL noise:\n%s", out)
	}
}

func TestRenderSummary_MultiplePackages(t *testing.T) {
	packages := []*Package{
		{
			Path: "pkg/a",
			Nodes: []*Node{{
				Kind:     KindTest,
				Display:  "TestA",
				Status:   StatusFail,
				Duration: time.Millisecond,
				Output:   []string{"    a_test.go:1: boom\n"},
			}},
		},
		{
			Path: "pkg/b",
			Nodes: []*Node{{
				Kind:     KindTest,
				Display:  "TestB",
				Status:   StatusFail,
				Duration: time.Millisecond,
				Output:   []string{"    b_test.go:2: crash\n"},
			}},
		},
	}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "pkg/a") {
		t.Errorf("expected first package, got:\n%s", out)
	}
	if !strings.Contains(out, "pkg/b") {
		t.Errorf("expected second package, got:\n%s", out)
	}
}

func TestRenderMarkdownSummary_AllPass(t *testing.T) {
	packages := []*Package{{
		Path:     "p",
		Duration: 500 * time.Millisecond,
		Nodes: []*Node{
			{Kind: KindTest, Display: "A", Status: StatusPass},
			{Kind: KindTest, Display: "B", Status: StatusPass},
		},
	}}

	var buf bytes.Buffer
	RenderMarkdownSummary(&buf, packages)
	out := buf.String()

	if !strings.Contains(out, "All 2 tests passed") {
		t.Errorf("expected success heading, got:\n%s", out)
	}
}

func TestRenderMarkdownSummary_WithFailures(t *testing.T) {
	packages := []*Package{{
		Path: "pkg/foo",
		Nodes: []*Node{
			{Kind: KindTest, Display: "Good", Status: StatusPass},
			{
				Kind:     KindTest,
				Display:  "Bad",
				Status:   StatusFail,
				Duration: 100 * time.Millisecond,
				Output:   []string{"    foo_test.go:10: want 1, got 2\n"},
			},
		},
	}}

	var buf bytes.Buffer
	RenderMarkdownSummary(&buf, packages)
	out := buf.String()

	if !strings.Contains(out, "1 of 2 tests failed") {
		t.Errorf("expected failure heading, got:\n%s", out)
	}
	if !strings.Contains(out, "<details>") {
		t.Errorf("expected collapsible section, got:\n%s", out)
	}
	if !strings.Contains(out, "pkg/foo") {
		t.Errorf("expected package name, got:\n%s", out)
	}
	if !strings.Contains(out, "want 1, got 2") {
		t.Errorf("expected error output, got:\n%s", out)
	}
}

func TestCollectFailures_DeepHierarchy(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Suite",
			Children: []*Node{{
				Kind:    KindMethod,
				Display: "Method",
				Children: []*Node{
					{
						Kind:    KindBlock,
						Display: "when valid",
						Children: []*Node{
							{Kind: KindBlock, Display: "returns ok", Status: StatusPass},
							{Kind: KindBlock, Display: "logs event", Status: StatusFail, Output: []string{"err"}},
						},
					},
					{Kind: KindBlock, Display: "when invalid", Status: StatusPass},
				},
			}},
		}},
	}}

	failures := collectFailures(packages)

	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}

	f := failures[0]
	got := strings.Join(f.Display, " / ")
	want := "Suite / Method / when valid / logs event"
	if got != want {
		t.Errorf("display path = %q, want %q", got, want)
	}
}

func TestRenderSummary_PackageDiagnostic(t *testing.T) {
	packages := []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{
			{Kind: KindTest, Display: "Foo", Status: StatusPass, Duration: time.Millisecond},
		},
		Output: []string{
			"==================\n",
			"WARNING: DATA RACE\n",
			"Write at 0x00c by goroutine 9:\n",
			"  pkg.TestFoo.func1()\n",
			"      foo_test.go:12 +0x38\n",
			"==================\n",
			"Found 1 data race(s)\n",
		},
	}}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "WARNING: DATA RACE") {
		t.Errorf("expected race warning in output:\n%s", out)
	}
	if !strings.Contains(out, "example.com/pkg") {
		t.Errorf("expected package path in output:\n%s", out)
	}
	if strings.Contains(out, "1 tests passed") {
		t.Errorf("should not say all passed when package failed:\n%s", out)
	}
}

func TestRenderMarkdownSummary_PackageDiagnostic(t *testing.T) {
	packages := []*Package{{
		Path:   "example.com/pkg",
		Status: StatusFail,
		Nodes: []*Node{
			{Kind: KindTest, Display: "Foo", Status: StatusPass, Duration: time.Millisecond},
		},
		Output: []string{
			"==================\n",
			"WARNING: DATA RACE\n",
			"Found 1 data race(s)\n",
		},
	}}

	var buf bytes.Buffer
	RenderMarkdownSummary(&buf, packages)
	out := buf.String()

	if !strings.Contains(out, "WARNING: DATA RACE") {
		t.Errorf("expected race warning in markdown:\n%s", out)
	}
	if !strings.Contains(out, "<details>") {
		t.Errorf("expected collapsible section:\n%s", out)
	}
}

func TestRenderSummary_BothTestFailureAndPackageDiagnostic(t *testing.T) {
	packages := []*Package{{
		Path:   "p",
		Status: StatusFail,
		Nodes: []*Node{
			{
				Kind: KindTest, Display: "Good", Status: StatusPass, Duration: time.Millisecond,
			},
			{
				Kind: KindTest, Display: "Bad", Status: StatusFail, Duration: time.Millisecond,
				Output: []string{"    foo_test.go:10: assertion failed\n"},
			},
		},
		Output: []string{
			"WARNING: DATA RACE\n",
			"Found 1 data race(s)\n",
		},
	}}

	var buf bytes.Buffer
	RenderSummary(&buf, packages, WithNoColor())
	out := buf.String()

	if !strings.Contains(out, "assertion failed") {
		t.Errorf("expected test failure output:\n%s", out)
	}
	if !strings.Contains(out, "WARNING: DATA RACE") {
		t.Errorf("expected race warning:\n%s", out)
	}
}
