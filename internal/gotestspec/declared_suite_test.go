package gotestspec_test

import (
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeclaredTestSuite covers what Overlay pairs that a single package's run
// cannot show: same-named suites of a package's internal and external test
// files, fuzz targets and their seeds, a package the stream never names and
// one only the stream holds.
type DeclaredTestSuite struct{}

func (s *DeclaredTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// declaredOf builds a declared package the way the static reader does: nodes
// named as go test names them, classified by the tree's rules.
func declaredOf(path string, nodes ...*gotestspec.Node) *gotestspec.Package {
	gotestspec.ClassifyRoots(nodes)
	return &gotestspec.Package{Path: path, Nodes: nodes}
}

func method(name string, children ...*gotestspec.Node) *gotestspec.Node {
	return &gotestspec.Node{Name: name, Line: 7, File: "unit_test.go", Children: children}
}

func (s *DeclaredTestSuite) TestSameNamedSuitesPairInRunOrder(t *gotest.T) {
	observed := treeOf(t, `{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate"}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove"}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove","Elapsed":0.01}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"fail","Package":"example.com/unit","Elapsed":0.05}`)
	declared := []*gotestspec.Package{declaredOf("example.com/unit",
		&gotestspec.Node{Name: "TestUnitTestSuite", Children: []*gotestspec.Node{method("TestCreate"), method("TestUpdate")}},
		&gotestspec.Node{Name: "TestUnitTestSuite", Children: []*gotestspec.Node{method("TestRemove")}},
	)}

	tree := gotestspec.Overlay(declared, observed)
	gotest.Len(t, tree, 1)
	gotest.Len(t, tree[0].Nodes, 2)
	internal, external := tree[0].Nodes[0], tree[0].Nodes[1]
	gotest.Equal(t, gotestspec.StatusPass, internal.Status)
	gotest.False(t, internal.External)
	gotest.Equal(t, gotestspec.StatusPass, internal.Children[0].Status)
	gotest.True(t, internal.Children[1].NotRun)
	gotest.Equal(t, gotestspec.StatusFail, external.Status)
	gotest.True(t, external.External)
	gotest.Equal(t, 2, external.Variant)
	gotest.Equal(t, gotestspec.StatusFail, external.Children[0].Status)
	gotest.Equal(t, "unit_test.go", external.Children[0].File)
	stats := gotestspec.CollectStats(tree)
	gotest.Equal(t, 1, stats.Passed)
	gotest.Equal(t, 1, stats.Failed)
	gotest.Equal(t, 1, stats.NotRun)
}

func (s *DeclaredTestSuite) TestFuzzTargetsTakeTheirWrapper(t *gotest.T) {
	observed := treeOf(t, `{"Action":"run","Package":"example.com/codec","Test":"FuzzCodecTestSuite_FuzzRoundTrip"}
{"Action":"run","Package":"example.com/codec","Test":"FuzzCodecTestSuite_FuzzRoundTrip/seed#0"}
{"Action":"pass","Package":"example.com/codec","Test":"FuzzCodecTestSuite_FuzzRoundTrip/seed#0","Elapsed":0}
{"Action":"pass","Package":"example.com/codec","Test":"FuzzCodecTestSuite_FuzzRoundTrip","Elapsed":0.01}
{"Action":"pass","Package":"example.com/codec","Elapsed":0.02}`)
	declared := []*gotestspec.Package{declaredOf("example.com/codec",
		&gotestspec.Node{Name: "TestCodecTestSuite", Children: []*gotestspec.Node{
			method("TestEncode"),
			{Kind: gotestspec.KindFuzz, Name: "FuzzRoundTrip", Display: "RoundTrip"},
			{Kind: gotestspec.KindFuzz, Name: "FuzzDecode", Display: "Decode"},
		}},
	)}

	tree := gotestspec.Overlay(declared, observed)
	suite := tree[0].Nodes[0]
	gotest.Len(t, suite.Children, 3)
	roundTrip, decode := suite.Children[1], suite.Children[2]
	gotest.Equal(t, gotestspec.StatusPass, roundTrip.Status)
	gotest.Len(t, roundTrip.Children, 1)
	gotest.True(t, decode.NotRun)
	stats := gotestspec.CollectStats(tree)
	gotest.Equal(t, 2, stats.Fuzzers)
	gotest.Equal(t, 1, stats.Passed)
	gotest.Equal(t, 2, stats.NotRun)
}

func (s *DeclaredTestSuite) TestPackagesOnEitherSideAlone(t *gotest.T) {
	observed := treeOf(t, `{"Action":"run","Package":"example.com/ran","Test":"TestRanTestSuite"}
{"Action":"pass","Package":"example.com/ran","Test":"TestRanTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"example.com/ran","Elapsed":0.02}`)
	declared := []*gotestspec.Package{declaredOf("example.com/idle",
		&gotestspec.Node{Name: "TestIdleTestSuite", Children: []*gotestspec.Node{method("TestWait")}},
	)}

	tree := gotestspec.Overlay(declared, observed)
	gotest.Len(t, tree, 2)
	idle, ran := tree[0], tree[1]
	gotest.Equal(t, "example.com/idle", idle.Path)
	gotest.Equal(t, gotestspec.StatusNone, idle.Status)
	gotest.True(t, idle.Nodes[0].NotRun)
	gotest.True(t, idle.Nodes[0].Children[0].NotRun)
	gotest.Equal(t, "example.com/ran", ran.Path)
	gotest.Equal(t, gotestspec.StatusPass, ran.Nodes[0].Status)
	gotest.False(t, gotestspec.HasFailures(tree))
}

func (s *DeclaredTestSuite) TestASuiteOfBenchmarksOnlyIsLeftOut(t *gotest.T) {
	declared := []*gotestspec.Package{declaredOf("example.com/bench",
		&gotestspec.Node{Name: "TestDispatchBenchTestSuite"},
		&gotestspec.Node{Name: "TestDispatchTestSuite", Children: []*gotestspec.Node{method("TestSend")}},
	)}

	tree := gotestspec.Overlay(declared, nil)
	gotest.Len(t, tree, 1)
	gotest.Len(t, tree[0].Nodes, 1)
	gotest.Equal(t, "TestDispatchTestSuite", tree[0].Nodes[0].Name)
	gotest.Equal(t, 1, gotestspec.CollectStats(tree).NotRun)
}

func (s *DeclaredTestSuite) TestALoneExternalSuitePairsWithTheRun(t *gotest.T) {
	observed := treeOf(t, `{"Action":"run","Package":"example.com/ext","Test":"TestExtTestSuite"}
{"Action":"run","Package":"example.com/ext","Test":"TestExtTestSuite/TestCreate"}
{"Action":"pass","Package":"example.com/ext","Test":"TestExtTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/ext","Test":"TestExtTestSuite","Elapsed":0.02}
{"Action":"pass","Package":"example.com/ext","Elapsed":0.05}`)
	declared := []*gotestspec.Package{declaredOf("example.com/ext",
		&gotestspec.Node{Name: "TestExtTestSuite", External: true, Children: []*gotestspec.Node{method("TestCreate")}},
	)}

	tree := gotestspec.Overlay(declared, observed)
	gotest.Len(t, tree[0].Nodes, 1)
	suite := tree[0].Nodes[0]
	gotest.Equal(t, gotestspec.StatusPass, suite.Status)
	gotest.True(t, suite.External)
	gotest.Equal(t, gotestspec.StatusPass, suite.Children[0].Status)
}

func (s *DeclaredTestSuite) TestTwinsPairInternalFirst(t *gotest.T) {
	observed := treeOf(t, `{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate"}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite/TestCreate","Elapsed":0.01}
{"Action":"pass","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite"}
{"Action":"run","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove"}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite/TestRemove","Elapsed":0.01}
{"Action":"fail","Package":"example.com/unit","Test":"TestUnitTestSuite","Elapsed":0.02}
{"Action":"fail","Package":"example.com/unit","Elapsed":0.05}`)
	declared := []*gotestspec.Package{declaredOf("example.com/unit",
		&gotestspec.Node{Name: "TestUnitTestSuite", External: true, Variant: 2, Children: []*gotestspec.Node{method("TestRemove")}},
		&gotestspec.Node{Name: "TestUnitTestSuite", Children: []*gotestspec.Node{method("TestCreate")}},
	)}

	tree := gotestspec.Overlay(declared, observed)
	gotest.Len(t, tree[0].Nodes, 2)
	internal, external := tree[0].Nodes[0], tree[0].Nodes[1]
	gotest.False(t, internal.External)
	gotest.Equal(t, gotestspec.StatusPass, internal.Status)
	gotest.Equal(t, "TestCreate", internal.Children[0].Name)
	gotest.True(t, external.External)
	gotest.Equal(t, 2, external.Variant)
	gotest.Equal(t, gotestspec.StatusFail, external.Status)
}
