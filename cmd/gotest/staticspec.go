package main

import (
	"fmt"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// buildStaticSpec assembles the specification tree from source alone, without
// running anything. It produces the same shape a test stream would, so every
// renderer and consumer works unchanged; the difference is that no node has a
// status, because nothing was executed.
//
// Where a method's behaviors cannot be enumerated from source, the walker
// reports it and the caller says so, rather than presenting a partial tree as
// exhaustive.
func buildStaticSpec(loadResults []*gotestgen.LoadResult, broken []gotestgen.BrokenPackage) ([]*gotestspec.Package, []string) {
	var out []*gotestspec.Package
	var notes []string

	for i := range broken {
		out = append(out, &gotestspec.Package{
			Path:   broken[i].PkgPath,
			Status: gotestspec.StatusFail,
			Output: broken[i].Errors,
		})
	}

	collector := gotestgen.NewCollector()

	for _, lr := range loadResults {
		pkgNode := &gotestspec.Package{Path: lr.PkgPath}

		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			result := collector.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				continue
			}
			for _, suite := range result.Suites {
				suiteNode, suiteNotes := staticSuiteNode(suite)
				if suiteNode == nil {
					continue
				}
				pkgNode.Nodes = append(pkgNode.Nodes, suiteNode)
				notes = append(notes, suiteNotes...)
			}
		}

		if len(pkgNode.Nodes) > 0 {
			sort.SliceStable(pkgNode.Nodes, func(a, b int) bool {
				return pkgNode.Nodes[a].Name < pkgNode.Nodes[b].Name
			})
			gotestspec.ClassifyRoots(pkgNode.Nodes)
			out = append(out, pkgNode)
		}
	}

	sort.SliceStable(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, notes
}

func staticSuiteNode(suite *gotestast.TestSuiteSpec) (*gotestspec.Node, []string) {
	var notes []string

	// The runtime tree keys suites by the generated entrypoint name, so the
	// static tree has to use the identical one to line up with it.
	node := &gotestspec.Node{Name: "Test" + suite.Identifier()}

	for _, method := range suite.TestCases() {
		methodName := method.Identifier()
		methodNode := &gotestspec.Node{Name: methodName}

		spec := suite.BehaviorsOf(method)
		methodNode.Children = staticBehaviorNodes(spec.Behaviors)
		if !spec.Complete {
			for _, note := range spec.Notes {
				notes = append(notes, suite.Identifier()+"."+methodName+": "+note)
			}
		}
		node.Children = append(node.Children, methodNode)
	}

	return node, notes
}

func staticBehaviorNodes(behaviors []*gotestast.Behavior) []*gotestspec.Node {
	var out []*gotestspec.Node
	for _, b := range behaviors {
		node := &gotestspec.Node{Name: b.Name}
		node.Children = staticBehaviorNodes(b.Children)
		out = append(out, node)
	}
	return out
}

// runStaticSpec renders the specification from source without running the
// suites. Nothing carries a verdict, because nothing executed; the exit code
// reports only whether the source could be read.
func runStaticSpec(ownArgs, goTestArgs []string, projectConfig *config.ProjectConfig, format, output string, noColor bool) int {
	cfg, err := parseExecFlags(ownArgs, goTestArgs, projectConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)

	loaded, broken, err := gotestgen.LoadPackagesForDiscovery(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	tree, notes := buildStaticSpec(loaded, broken)

	w := os.Stdout
	if output != "" {
		f, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer f.Close()
		w = f
	}

	var renderOpts []gotestspec.RenderOption
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	case "json":
		gotestspec.RenderJSON(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree, renderOpts...)
	}

	// Behaviors the walker could not enumerate are reported rather than
	// silently omitted: an incomplete specification that says so is useful,
	// one that pretends to be complete is not.
	for _, note := range notes {
		fmt.Fprintf(os.Stderr, "incomplete: %s\n", note)
	}

	if len(broken) > 0 {
		return 2
	}
	return 0
}
