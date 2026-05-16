package gotestspec

import (
	"bytes"
	"encoding/json"
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
