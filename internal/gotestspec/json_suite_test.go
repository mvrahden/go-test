package gotestspec_test

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// JSONRenderTestSuite covers the JSON rendering of a tree: hierarchy, stats,
// focus and exclusion flags, and test and package output.
type JSONRenderTestSuite struct{}

func (s *JSONRenderTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type jsonRenderCtx struct{}

func (s *JSONRenderTestSuite) BeforeEach(t *gotest.T) *jsonRenderCtx { return &jsonRenderCtx{} }

func renderJSON(t *gotest.T, packages []*gotestspec.Package) gotestspec.ExportJSONRoot {
	var buf bytes.Buffer
	gotestspec.RenderJSON(&buf, packages)
	var result gotestspec.ExportJSONRoot
	gotest.NoError(t, json.Unmarshal(buf.Bytes(), &result), "invalid JSON: %s", buf.String())
	return result
}

func (s *JSONRenderTestSuite) TestRenderJSON_SuiteHierarchy(t *gotest.T, _ *jsonRenderCtx) {
	result := renderJSON(t, []*gotestspec.Package{{
		Path:     "example.com/pkg",
		Status:   gotestspec.StatusPass,
		Duration: 500 * time.Millisecond,
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "UserService",
			Children: []*gotestspec.Node{{
				Kind:    gotestspec.KindMethod,
				Display: "Create",
				Children: []*gotestspec.Node{{
					Kind:     gotestspec.KindBlock,
					Display:  "returns ok",
					Status:   gotestspec.StatusPass,
					Duration: 8 * time.Millisecond,
				}},
			}},
		}},
	}})

	gotest.Len(t, result.Packages, 1)
	pkg := result.Packages[0]
	gotest.Equal(t, "example.com/pkg", pkg.Path)
	gotest.Equal(t, "pass", pkg.Status)
	gotest.Equal(t, 0.5, pkg.Duration)

	gotest.Len(t, pkg.Nodes, 1)
	suite := pkg.Nodes[0]
	gotest.Equal(t, "UserService", suite.Display)
	gotest.Equal(t, "suite", suite.Kind)

	method := suite.Children[0]
	gotest.Equal(t, "method", method.Kind)

	leaf := method.Children[0]
	gotest.Equal(t, "returns ok", leaf.Display)
	gotest.Equal(t, "pass", leaf.Status)
	gotest.Equal(t, 0.008, leaf.Duration)
}

func (s *JSONRenderTestSuite) TestRenderJSON_IncludesStats(t *gotest.T, _ *jsonRenderCtx) {
	result := renderJSON(t, []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{
			{
				Kind:    gotestspec.KindSuite,
				Display: "Foo",
				Children: []*gotestspec.Node{
					{Kind: gotestspec.KindMethod, Display: "A", Status: gotestspec.StatusPass, Duration: time.Millisecond},
					{Kind: gotestspec.KindMethod, Display: "B", Status: gotestspec.StatusFail, Duration: 2 * time.Millisecond},
				},
			},
			{Kind: gotestspec.KindTest, Display: "Helper", Status: gotestspec.StatusPass, Duration: time.Millisecond},
		},
	}})

	gotest.Equal(t, 1, result.Stats.Suites)
	gotest.Equal(t, 2, result.Stats.Behaviors)
	gotest.Equal(t, 1, result.Stats.Tests)
	gotest.Equal(t, 2, result.Stats.Passed)
	gotest.Equal(t, 1, result.Stats.Failed)
}

func (s *JSONRenderTestSuite) TestRenderJSON_FocusedAndExcluded(t *gotest.T, _ *jsonRenderCtx) {
	result := renderJSON(t, []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindSuite, Display: "Focused", Focused: true, Status: gotestspec.StatusPass},
			{Kind: gotestspec.KindSuite, Display: "Excluded", Excluded: true, Status: gotestspec.StatusSkip},
		},
	}})

	gotest.True(t, result.Packages[0].Nodes[0].Focused)
	gotest.True(t, result.Packages[0].Nodes[1].Excluded)
}

func (s *JSONRenderTestSuite) TestRenderJSON_ErrorOutput(t *gotest.T, _ *jsonRenderCtx) {
	result := renderJSON(t, []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindTest,
			Display:  "Broken",
			Status:   gotestspec.StatusFail,
			Duration: time.Millisecond,
			Output:   []string{"expected 1, got 2\n"},
		}},
	}})

	gotest.Equal(t, []string{"expected 1, got 2\n"}, result.Packages[0].Nodes[0].Output)
}

func (s *JSONRenderTestSuite) TestRenderJSON_IncludesPackageOutput(t *gotest.T, _ *jsonRenderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderJSON(&buf, []*gotestspec.Package{{
		Path:   "p",
		Status: gotestspec.StatusFail,
		Nodes:  []*gotestspec.Node{{Kind: gotestspec.KindTest, Display: "Foo", Status: gotestspec.StatusPass}},
		Output: []string{"WARNING: DATA RACE\n"},
	}})

	gotest.Contains(t, buf.String(), "WARNING: DATA RACE", "package diagnostics belong in the JSON")
}

func benchmarkSuite() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Foo",
			Children: []*gotestspec.Node{
				{
					Kind:        gotestspec.KindBenchmark,
					Display:     "Parse",
					Status:      gotestspec.StatusPass,
					Iterations:  1201,
					NsPerOp:     985.2,
					BytesPerOp:  24,
					AllocsPerOp: 3,
				},
				{Kind: gotestspec.KindMethod, Display: "Regular", Status: gotestspec.StatusPass, Duration: time.Millisecond},
			},
		}},
	}}
}

func (s *JSONRenderTestSuite) TestRenderJSON_BenchmarkNode(t *gotest.T, _ *jsonRenderCtx) {
	var buf bytes.Buffer
	gotestspec.RenderJSON(&buf, benchmarkSuite())

	var raw map[string]any
	gotest.NoError(t, json.Unmarshal(buf.Bytes(), &raw), "invalid JSON: %s", buf.String())
	pkgs, _ := raw["packages"].([]any)
	nodes, _ := pkgs[0].(map[string]any)["nodes"].([]any)
	suite, _ := nodes[0].(map[string]any)
	children, _ := suite["children"].([]any)
	bench, _ := children[0].(map[string]any)
	regular, _ := children[1].(map[string]any)

	gotest.Equal(t, any("benchmark"), bench["kind"])
	benchKeys := []string{"ns_per_op", "bytes_per_op", "allocs_per_op", "iterations"}
	for _, key := range benchKeys {
		gotest.Contains(t, bench, key, "bench node carries %q", key)
	}
	gotest.Equal(t, any(985.2), bench["ns_per_op"])
	gotest.Equal(t, any(float64(24)), bench["bytes_per_op"])
	gotest.Equal(t, any(float64(3)), bench["allocs_per_op"])
	gotest.Equal(t, any(float64(1201)), bench["iterations"])
	for _, key := range benchKeys {
		gotest.NotContains(t, regular, key, "a non-bench node omits %q", key)
	}
}

func (s *JSONRenderTestSuite) TestRenderJSON_IncludesBenchmarkStats(t *gotest.T, _ *jsonRenderCtx) {
	result := renderJSON(t, []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:    gotestspec.KindSuite,
			Display: "Foo",
			Children: []*gotestspec.Node{
				{Kind: gotestspec.KindBenchmark, Display: "Parse", Status: gotestspec.StatusPass, Iterations: 10, NsPerOp: 1.0},
			},
		}},
	}})

	gotest.Equal(t, 1, result.Stats.Benchmarks)
	gotest.Equal(t, 0, result.Stats.Behaviors)
}
