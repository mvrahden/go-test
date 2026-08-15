package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StaticSpecTestSuite covers `spec --static`: the specification read from the
// source instead of from a run. The corpus is the walker's own testdata, which
// is written to be adverse — tables, duplicates, slashes, and behaviors the
// walker deliberately cannot see.
type StaticSpecTestSuite struct {
	dir        string
	workDir    string
	overlayDir string
	corpus     string
	rendered   map[string]string
}

func (s *StaticSpecTestSuite) BeforeAll(t *gotest.T) {
	corpus, err := filepath.Abs(filepath.Join("..", "..", "internal", "gotestast", "testdata", "behaviors"))
	gotest.NoError(t, err)
	s.corpus = corpus

	s.workDir, err = os.MkdirTemp("", "gotest-staticspec-render-")
	gotest.NoError(t, err)

	s.rendered = map[string]string{}
	for _, format := range []string{"terminal", "md", "json"} {
		s.rendered[format] = s.render(t, format, corpus)
	}
}

func (s *StaticSpecTestSuite) AfterAll(t *gotest.T) {
	gotest.NoError(t, os.RemoveAll(s.workDir))
}

func (s *StaticSpecTestSuite) BeforeEach(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-staticspec-")
	gotest.NoError(t, err)
	s.dir = dir
	s.overlayDir = ""
}

func (s *StaticSpecTestSuite) AfterEach(t *gotest.T) {
	gotest.NoError(t, os.RemoveAll(s.dir))
	if s.overlayDir != "" {
		gotest.NoError(t, os.RemoveAll(s.overlayDir))
	}
}

// render runs the subcommand into a file, which is also how a caller publishes
// a specification, and returns what it wrote.
func (s *StaticSpecTestSuite) render(t *gotest.T, format, pattern string) string {
	out := filepath.Join(s.workDir, "spec."+format)
	code := ExportRunStaticSpec(nil, []string{pattern}, &config.ProjectConfig{}, format, out, true)
	gotest.Equal(t, 0, code, "spec --static --format=%s", format)

	data, err := os.ReadFile(out) //nolint:gosec // G304: path built in this test
	gotest.NoError(t, err)
	return string(data)
}

// The contract the whole feature rests on: where the walker reports the list is
// exhaustive, the names it predicts are the names go test actually produces.
// Anything else puts a behavior in the tree under a name no run will report.
func (s *StaticSpecTestSuite) TestPredictsTheNamesARunProduces(t *gotest.T) {
	t.When("the corpus is both read and run", func(w *gotest.T) {
		static := s.staticPaths(w)
		observed := s.observedPaths(w)

		w.It("predicts every complete method exactly", func(it *gotest.T) {
			for method, want := range static.complete {
				gotest.Equal(it, want, observed[method], "method %s", method)
			}
			gotest.NotEmpty(it, static.complete, "no complete methods in the corpus")
		})

		w.It("never claims a behavior a run does not report", func(it *gotest.T) {
			// An incomplete method may report more at run time — that is what
			// incomplete means — but never fewer, and never under other names.
			for method, declared := range static.incomplete {
				for _, path := range declared {
					gotest.Contains(it, observed[method], path, "method %s", method)
				}
			}
			gotest.NotEmpty(it, static.incomplete, "no incomplete methods in the corpus")
		})
	})
}

type staticTrees struct {
	complete   map[string][]string
	incomplete map[string][]string
}

// staticPaths renders the corpus as JSON and splits the methods by whether the
// walker admitted it could not see all of them.
func (s *StaticSpecTestSuite) staticPaths(t *gotest.T) staticTrees {
	var doc specDocument
	gotest.NoError(t, json.Unmarshal([]byte(s.rendered["json"]), &doc))

	trees := staticTrees{complete: map[string][]string{}, incomplete: map[string][]string{}}
	for _, pkg := range doc.Packages {
		for _, suite := range pkg.Nodes {
			for _, method := range suite.Children {
				bucket := trees.complete
				if method.Incomplete {
					bucket = trees.incomplete
				}
				bucket[method.Name] = nodePaths(method.Children, "")
			}
		}
	}
	return trees
}

// observedPaths runs the corpus for real and reads the same paths back out of
// the tree the stream produces.
func (s *StaticSpecTestSuite) observedPaths(t *gotest.T) map[string][]string {
	loaded, _, err := gotestgen.LoadPackages([]string{s.corpus}, nil)
	gotest.NoError(t, err)
	results, _, err := gotestgen.GenerateFromLoaded(loaded)
	gotest.NoError(t, err)

	overlayDir, err := gotestrunner.WriteOverlay(results)
	gotest.NoError(t, err)
	s.overlayDir = overlayDir

	cmd := exec.CommandContext(context.Background(), "go", //nolint:gosec // G204: go tool with controlled arguments
		"test", "-json", "-ldflags=-checklinkname=0",
		"-overlay="+filepath.Join(overlayDir, "overlay.json"), s.corpus)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{Grace: gotestrunner.GraceKill})
	gotest.NoError(t, mp.Start())
	_ = mp.WaitWithGrace(context.Background())

	events, err := gotestspec.ParseEvents(bytes.NewReader(stdout.Bytes()))
	gotest.NoError(t, err)
	tree := gotestspec.BuildTree(events)

	observed := map[string][]string{}
	for _, pkg := range tree {
		for _, suite := range pkg.Nodes {
			for _, method := range suite.Children {
				observed[method.Name] = specNodePaths(method.Children, "")
			}
		}
	}
	gotest.NotEmpty(t, observed, "the corpus produced no test events")
	return observed
}

func (s *StaticSpecTestSuite) TestCarriesNoVerdicts(t *gotest.T) {
	t.When("the terminal spec is rendered from source", func(w *gotest.T) {
		out := s.rendered["terminal"]

		w.It("shows no status glyph", func(it *gotest.T) {
			gotest.NotContains(it, out, "✓")
			gotest.NotContains(it, out, "✗")
		})

		w.It("shows no duration", func(it *gotest.T) {
			gotest.NotContains(it, out, "ms)")
		})

		w.It("counts what it read without claiming a verdict", func(it *gotest.T) {
			gotest.Contains(it, out, "behaviors")
			gotest.NotContains(it, out, "passed")
			// A count line ending in a colon reads as a line that lost its tail.
			gotest.NotContains(it, out, "behaviors: \n")
		})
	})

	t.When("the markdown spec is rendered from source", func(w *gotest.T) {
		out := s.rendered["md"]

		w.It("drops the Status and Duration columns", func(it *gotest.T) {
			gotest.Contains(it, out, "| Behavior |")
			gotest.NotContains(it, out, "| Behavior | Status | Duration |")
			// The duration of something that never ran is not "<1ms".
			gotest.NotContains(it, out, "<1ms")
		})

		w.It("says nothing was executed instead of reporting zero of each", func(it *gotest.T) {
			gotest.Contains(it, out, "nothing was executed")
			gotest.NotContains(it, out, "0 passed, 0 failed, 0 skipped")
		})
	})

	t.When("the JSON spec is rendered from source", func(w *gotest.T) {
		var doc specDocument
		gotest.NoError(w, json.Unmarshal([]byte(s.rendered["json"]), &doc))

		w.It("leaves every node without a status", func(it *gotest.T) {
			for _, pkg := range doc.Packages {
				for _, node := range pkg.Nodes {
					assertStatusNone(it, node)
				}
			}
		})
	})
}

// A partial list that presents itself as whole is worse than no list, so every
// surface has to carry the admission — not only the stderr notes.
func (s *StaticSpecTestSuite) TestDeclaresIncompletenessOnEverySurface(t *gotest.T) {
	t.When("a method declares behaviors the walker cannot see", func(w *gotest.T) {
		w.It("marks it in the terminal tree", func(it *gotest.T) {
			gotest.Contains(it, s.rendered["terminal"], "Conditional — INCOMPLETE")
		})

		w.It("marks it in markdown", func(it *gotest.T) {
			gotest.Contains(it, s.rendered["md"], "_Incomplete:")
		})

		w.It("marks a method whose behaviors are all runtime-dependent", func(it *gotest.T) {
			// It has no table of its own, so it appears as a row of its suite's.
			// Unmarked it reads as a behavior that simply has nothing beneath it.
			gotest.Contains(it, s.rendered["md"],
				"| NonLiteralTable — _incomplete: behaviors known only at run time_ |")
		})

		w.It("marks it in JSON", func(it *gotest.T) {
			var doc specDocument
			gotest.NoError(it, json.Unmarshal([]byte(s.rendered["json"]), &doc))
			gotest.True(it, findIncomplete(doc, "TestConditional"),
				"TestConditional is not marked incomplete in JSON")
			gotest.False(it, findIncomplete(doc, "TestNesting"),
				"a fully declared method must not be marked incomplete")
		})
	})
}

func (s *StaticSpecTestSuite) TestExitCodes(t *gotest.T) {
	t.When("the source can be read", func(w *gotest.T) {
		w.It("exits 0", func(it *gotest.T) {
			out := filepath.Join(s.dir, "ok.txt")
			gotest.Equal(it, 0, ExportRunStaticSpec(nil, []string{s.corpus}, &config.ProjectConfig{}, "terminal", out, true))
		})
	})

	t.When("a package does not compile", func(w *gotest.T) {
		broken := filepath.Join(s.dir, "broken")
		gotest.NoError(w, os.MkdirAll(broken, 0o750))
		gotest.NoError(w, os.WriteFile(filepath.Join(broken, "x_test.go"),
			[]byte("package broken\n\nfunc Nope() { undefinedSymbol() }\n"), 0o600))

		w.It("exits 2 rather than reporting an empty specification", func(it *gotest.T) {
			out := filepath.Join(s.dir, "broken.txt")
			gotest.Equal(it, 2, ExportRunStaticSpec(nil, []string{broken}, &config.ProjectConfig{}, "terminal", out, true))
		})
	})

	t.When("the output file cannot be created", func(w *gotest.T) {
		w.It("exits 2", func(it *gotest.T) {
			out := filepath.Join(s.dir, "no-such-dir", "spec.txt")
			gotest.Equal(it, 2, ExportRunStaticSpec(nil, []string{s.corpus}, &config.ProjectConfig{}, "terminal", out, true))
		})
	})
}

// --static reads the specification in the source, --input the one in a recorded
// run. Honouring one silently while the caller asked for both hands back a tree
// they did not ask for.
func (s *StaticSpecTestSuite) TestStaticAndInputAreMutuallyExclusive(t *gotest.T) {
	t.It("rejects the pair instead of picking one", func(it *gotest.T) {
		code := ExportRunSpec(Invocation{Args: []string{"--static", "--input=events.json"}})
		gotest.Equal(it, 2, code)
	})
}

type specDocument struct {
	Packages []struct {
		Path  string     `json:"path"`
		Nodes []specNode `json:"nodes"`
	} `json:"packages"`
}

type specNode struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Incomplete bool       `json:"incomplete"`
	Children   []specNode `json:"children"`
}

func nodePaths(nodes []specNode, prefix string) []string {
	var out []string
	for _, n := range nodes {
		path := prefix + n.Name
		out = append(out, path)
		out = append(out, nodePaths(n.Children, path+"/")...)
	}
	return out
}

func specNodePaths(nodes []*gotestspec.Node, prefix string) []string {
	var out []string
	for _, n := range nodes {
		path := prefix + n.Name
		out = append(out, path)
		out = append(out, specNodePaths(n.Children, path+"/")...)
	}
	return out
}

func assertStatusNone(t *gotest.T, n specNode) {
	gotest.Equal(t, "none", n.Status, "node %s carries a verdict", n.Name)
	for i := range n.Children {
		assertStatusNone(t, n.Children[i])
	}
}

func findIncomplete(doc specDocument, method string) bool {
	for _, pkg := range doc.Packages {
		for _, suite := range pkg.Nodes {
			for _, node := range suite.Children {
				if node.Name == method {
					return node.Incomplete
				}
			}
		}
	}
	return false
}
