package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRenderJSON_SuiteHierarchy(t *testing.T) {
	packages := []*Package{{
		Path:     "example.com/pkg",
		Status:   StatusPass,
		Duration: 500 * time.Millisecond,
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
	RenderJSON(&buf, packages)

	var result jsonRoot
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	if len(result.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(result.Packages))
	}
	pkg := result.Packages[0]
	if pkg.Path != "example.com/pkg" {
		t.Errorf("path = %q", pkg.Path)
	}
	if pkg.Status != "pass" {
		t.Errorf("status = %q", pkg.Status)
	}
	if pkg.Duration != 0.5 {
		t.Errorf("duration = %f, want 0.5", pkg.Duration)
	}

	if len(pkg.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(pkg.Nodes))
	}
	suite := pkg.Nodes[0]
	if suite.Display != "UserService" {
		t.Errorf("display = %q", suite.Display)
	}
	if suite.Kind != "suite" {
		t.Errorf("kind = %q", suite.Kind)
	}

	method := suite.Children[0]
	if method.Kind != "method" {
		t.Errorf("method kind = %q", method.Kind)
	}

	leaf := method.Children[0]
	if leaf.Display != "returns ok" {
		t.Errorf("leaf display = %q", leaf.Display)
	}
	if leaf.Status != "pass" {
		t.Errorf("leaf status = %q", leaf.Status)
	}
	if leaf.Duration != 0.008 {
		t.Errorf("leaf duration = %f, want 0.008", leaf.Duration)
	}
}

func TestRenderJSON_IncludesStats(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{
			{
				Kind:    KindSuite,
				Display: "Foo",
				Children: []*Node{
					{Kind: KindMethod, Display: "A", Status: StatusPass, Duration: time.Millisecond},
					{Kind: KindMethod, Display: "B", Status: StatusFail, Duration: 2 * time.Millisecond},
				},
			},
			{Kind: KindTest, Display: "Helper", Status: StatusPass, Duration: time.Millisecond},
		},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)

	var result jsonRoot
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	if result.Stats.Suites != 1 {
		t.Errorf("suites = %d, want 1", result.Stats.Suites)
	}
	if result.Stats.Behaviors != 2 {
		t.Errorf("behaviors = %d, want 2", result.Stats.Behaviors)
	}
	if result.Stats.Tests != 1 {
		t.Errorf("tests = %d, want 1", result.Stats.Tests)
	}
	if result.Stats.Passed != 2 {
		t.Errorf("passed = %d, want 2", result.Stats.Passed)
	}
	if result.Stats.Failed != 1 {
		t.Errorf("failed = %d, want 1", result.Stats.Failed)
	}
}

func TestRenderJSON_FocusedAndExcluded(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{
			{Kind: KindSuite, Display: "Focused", Focused: true, Status: StatusPass},
			{Kind: KindSuite, Display: "Excluded", Excluded: true, Status: StatusSkip},
		},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)

	var result jsonRoot
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	if !result.Packages[0].Nodes[0].Focused {
		t.Error("expected first node to be focused")
	}
	if !result.Packages[0].Nodes[1].Excluded {
		t.Error("expected second node to be excluded")
	}
}

func TestRenderJSON_ErrorOutput(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:     KindTest,
			Display:  "Broken",
			Status:   StatusFail,
			Duration: time.Millisecond,
			Output:   []string{"expected 1, got 2\n"},
		}},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)

	var result jsonRoot
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	node := result.Packages[0].Nodes[0]
	if len(node.Output) != 1 {
		t.Fatalf("expected 1 output line, got %d", len(node.Output))
	}
	if node.Output[0] != "expected 1, got 2\n" {
		t.Errorf("output = %q", node.Output[0])
	}
}

func TestRenderJSON_BenchmarkNode(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Foo",
			Children: []*Node{
				{
					Kind:        KindBenchmark,
					Display:     "Parse",
					Status:      StatusPass,
					Iterations:  1201,
					NsPerOp:     985.2,
					BytesPerOp:  24,
					AllocsPerOp: 3,
				},
				{Kind: KindMethod, Display: "Regular", Status: StatusPass, Duration: time.Millisecond},
			},
		}},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	pkgs, _ := raw["packages"].([]any)
	nodes, _ := pkgs[0].(map[string]any)["nodes"].([]any)
	suite, _ := nodes[0].(map[string]any)
	children, _ := suite["children"].([]any)
	bench, _ := children[0].(map[string]any)
	regular, _ := children[1].(map[string]any)

	if bench["kind"] != "benchmark" {
		t.Errorf("kind = %v, want benchmark", bench["kind"])
	}

	benchKeys := []string{"ns_per_op", "bytes_per_op", "allocs_per_op", "iterations"}
	for _, key := range benchKeys {
		if _, ok := bench[key]; !ok {
			t.Errorf("bench node missing key %q, got: %v", key, bench)
		}
	}
	if bench["ns_per_op"] != 985.2 {
		t.Errorf("ns_per_op = %v, want 985.2", bench["ns_per_op"])
	}
	if bench["bytes_per_op"] != float64(24) {
		t.Errorf("bytes_per_op = %v, want 24", bench["bytes_per_op"])
	}
	if bench["allocs_per_op"] != float64(3) {
		t.Errorf("allocs_per_op = %v, want 3", bench["allocs_per_op"])
	}
	if bench["iterations"] != float64(1201) {
		t.Errorf("iterations = %v, want 1201", bench["iterations"])
	}

	for _, key := range benchKeys {
		if _, ok := regular[key]; ok {
			t.Errorf("non-bench node should omit key %q, got present: %v", key, regular)
		}
	}
}

func TestRenderJSON_IncludesBenchmarkStats(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:    KindSuite,
			Display: "Foo",
			Children: []*Node{
				{Kind: KindBenchmark, Display: "Parse", Status: StatusPass, Iterations: 10, NsPerOp: 1.0},
			},
		}},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)

	var result jsonRoot
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}

	if result.Stats.Benchmarks != 1 {
		t.Errorf("benchmarks = %d, want 1", result.Stats.Benchmarks)
	}
	if result.Stats.Behaviors != 0 {
		t.Errorf("behaviors = %d, want 0", result.Stats.Behaviors)
	}
}

func TestRenderJSON_IncludesPackageOutput(t *testing.T) {
	packages := []*Package{{
		Path:   "p",
		Status: StatusFail,
		Nodes:  []*Node{{Kind: KindTest, Display: "Foo", Status: StatusPass}},
		Output: []string{"WARNING: DATA RACE\n"},
	}}

	var buf bytes.Buffer
	RenderJSON(&buf, packages)
	out := buf.String()

	if !strings.Contains(out, "WARNING: DATA RACE") {
		t.Errorf("JSON output should contain package diagnostics:\n%s", out)
	}
}
