package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
		pkgNode := &gotestspec.Package{Path: lr.PkgPath, Dir: lr.PkgDir}

		for _, pkg := range []*packages.Package{lr.Ptest, lr.Pxtest} {
			if pkg == nil {
				continue
			}
			result := collector.CollectSuiteSpecs(pkg)
			if len(result.Errs) > 0 {
				// Dropping the package silently would present "no suites here"
				// as a fact about the source rather than about the reader.
				for _, e := range result.Errs {
					notes = append(notes, fmt.Sprintf("%s: %s", pkg.PkgPath, e.Err))
				}
				continue
			}
			for _, suite := range result.Suites {
				suiteNode, suiteNotes := staticSuiteNode(suite)
				if suiteNode == nil {
					continue
				}
				if pkg == lr.Pxtest {
					suiteNode.External = true
					if hasSuiteNamed(pkgNode.Nodes, suiteNode.Name) {
						suiteNode.Variant = 2
					}
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
			carryExcluded(pkgNode.Nodes)
			out = append(out, pkgNode)
		}
	}

	sort.SliceStable(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, notes
}

// hasSuiteNamed reports whether nodes holds an internal suite of that name:
// the external suite of the same name is then the run's second.
func hasSuiteNamed(nodes []*gotestspec.Node, name string) bool {
	for _, n := range nodes {
		if !n.External && n.Name == name {
			return true
		}
	}
	return false
}

// carryExcluded carries an excluded suite's or method's flag down to every
// block under it: a reader of one node must not take a parked behavior for a
// live one.
func carryExcluded(nodes []*gotestspec.Node) {
	for _, n := range nodes {
		if n.Excluded {
			markExcluded(n.Children)
			continue
		}
		carryExcluded(n.Children)
	}
}

func markExcluded(nodes []*gotestspec.Node) {
	for _, n := range nodes {
		n.Excluded = true
		markExcluded(n.Children)
	}
}

func staticSuiteNode(suite *gotestast.TestSuiteSpec) (*gotestspec.Node, []string) {
	var notes []string

	// The runtime tree keys suites by the generated entrypoint name, so the
	// static tree has to use the identical one to line up with it. Positions
	// are the ones gotest discover gives the same nodes.
	fset := suite.Package().Fset
	pos := fset.Position(suite.TypeSpecPos())
	node := &gotestspec.Node{Name: "Test" + suite.Identifier(), File: filepath.Base(pos.Filename), Line: pos.Line}

	for _, method := range suite.TestCases() {
		methodName := method.Identifier()
		mPos := fset.Position(method.Pos())
		file := filepath.Base(mPos.Filename)
		methodNode := &gotestspec.Node{Name: methodName, File: file, Line: mPos.Line}

		spec := suite.BehaviorsOf(method)
		// A method's behaviors are read from its body, so they share its file.
		methodNode.Children = staticBehaviorNodes(spec.Behaviors, file)
		if !spec.Complete {
			// Carried on the node, not only on stderr: a spec written to a file
			// or piped as JSON has to state its own limits, or the reader has
			// no way to know the list is a floor.
			methodNode.Incomplete = true
			for _, note := range spec.Notes {
				notes = append(notes, suite.Identifier()+"."+methodName+": "+note)
			}
		}
		node.Children = append(node.Children, methodNode)
	}
	// Fuzz targets are properties the suite states; they sit beside its
	// examples, named as the run tree names them.
	for _, fz := range suite.Fuzzers() {
		fPos := fset.Position(fz.Pos())
		node.Children = append(node.Children, &gotestspec.Node{
			Kind:    gotestspec.KindFuzz,
			Name:    fz.Identifier(),
			Display: strings.TrimPrefix(fz.Identifier(), "Fuzz"),
			File:    filepath.Base(fPos.Filename),
			Line:    fPos.Line,
		})
	}

	return node, notes
}

func staticBehaviorNodes(behaviors []*gotestast.Behavior, file string) []*gotestspec.Node {
	var out []*gotestspec.Node
	for _, b := range behaviors {
		// The tree shows duplicates under the name the developer wrote.
		node := &gotestspec.Node{
			Name:        b.Written(),
			Vocab:       vocabOf(b.Kind),
			SourceLabel: b.Display,
			File:        file,
			Line:        b.Line,
		}
		node.Children = staticBehaviorNodes(b.Children, file)
		out = append(out, node)
	}
	return out
}

// runStaticSpec renders the specification from source without running the
// suites. Nothing carries a verdict, because nothing executed; the exit code
// reports only whether the source could be read.
func runStaticSpec(own OwnFlags, goTestArgs []string, projectConfig *config.ProjectConfig, format, output string, noColor bool) int {
	cfg, err := parseExecFlags(own, goTestArgs, projectConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)

	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	loaded, broken := pkgs.Loaded, pkgs.Broken
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

	// Nothing ran, so nothing has a verdict or a duration to show.
	renderOpts := []gotestspec.RenderOption{gotestspec.WithoutVerdicts()}
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree, append(renderOpts, reportOptions()...)...)
	case "json":
		gotestspec.RenderJSON(w, tree, gotestspec.WithoutVerdicts())
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
