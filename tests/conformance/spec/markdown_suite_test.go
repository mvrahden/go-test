package spec_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecMarkdownTestSuite: the Markdown spec of a run and of the source, over
// labels Markdown would read as syntax, an It with a slash and X_ exclusions.
type SpecMarkdownTestSuite struct {
	live   testkit.Run
	static testkit.Run
}

const fixture = "../testdata/specmarkdown/"

// SuiteConfig: the live spec runs the fixture through the whole pipeline.
func (s *SpecMarkdownTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SpecMarkdownTestSuite) BeforeAll(t *gotest.T) {
	var err error
	s.live, err = testkit.RunCLI(t.Context(), ".", "spec", "--format=md", fixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, s.live.Exit, "spec: %s", s.live.Stderr)
	s.static, err = testkit.RunCLI(t.Context(), ".", "spec", "--static", "--format=md", fixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, s.static.Exit, "spec --static: %s", s.static.Stderr)
}

func (s *SpecMarkdownTestSuite) AfterAll(t *gotest.T) {}

// cells counts the cells of a table row: the pipes no backslash escapes, less
// the closing one.
func cells(row string) int {
	n := 0
	for i := 0; i < len(row); i++ {
		switch row[i] {
		case '\\':
			i++
		case '|':
			n++
		}
	}
	return n - 1
}

// tableRows is every table row of a Markdown document, header and rule included.
func tableRows(md string) []string {
	var rows []string
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		}
	}
	return rows
}

func (s *SpecMarkdownTestSuite) TestLabels(t *gotest.T) {
	t.When("a label holds a pipe, a backslash, an angle bracket, a backtick or a line break", func(w *gotest.T) {
		w.It("keeps each behavior on one row of its table", func(it *gotest.T) {
			for _, doc := range []struct {
				name, md string
				width    int
				rows     []string
			}{
				{"spec", s.live.Stdout, 3, []string{
					"| **when a label holds a \\| b** | | |",
					"| &nbsp;&nbsp;keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text | PASS | ",
					"| &nbsp;&nbsp;keeps a\\\\b and a line break in one row | PASS | ",
					"| row \\| &lt;i&gt;x&lt;/i&gt; | PASS | ",
				}},
				{"spec --static", s.static.Stdout, 1, []string{
					"| **when a label holds a \\| b** |\n",
					"| &nbsp;&nbsp;keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text |\n",
					"| &nbsp;&nbsp;keeps a\\\\b and a line break in one row |\n",
					"| row \\| &lt;i&gt;x&lt;/i&gt; |\n",
				}},
			} {
				for _, row := range doc.rows {
					gotest.Contains(it, doc.md, row, "%s", doc.name)
				}
				for _, row := range tableRows(doc.md) {
					gotest.Equal(it, doc.width, cells(row), "%s row %q", doc.name, row)
				}
			}
		})

		w.It("renders the label as text, with no HTML tag and no code span", func(it *gotest.T) {
			for _, md := range []string{s.live.Stdout, s.static.Stdout} {
				for _, raw := range []string{"<b>", "</b>", "<i>", "</i>", "and `code`"} {
					gotest.NotContains(it, md, raw)
				}
				gotest.Contains(it, md, "keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text")
			}
		})
	})
}

func (s *SpecMarkdownTestSuite) TestLinkSyntax(t *gotest.T) {
	t.When("a label holds brackets, asterisks, underscores or an ampersand", func(w *gotest.T) {
		w.It("renders the label as written, with no link, emphasis or entity", func(it *gotest.T) {
			for _, md := range []string{s.live.Stdout, s.static.Stdout} {
				gotest.Contains(it, md, "**when a label holds a \\[link\\](http://x)**")
				gotest.Contains(it, md, "keeps \\*stars\\*, \\_underscores\\_ and &amp;#124; as written")
				gotest.NotContains(it, md, "[link](")
				gotest.NotContains(it, md, " *stars*")
				gotest.NotContains(it, md, " _underscores_")
				gotest.NotContains(it, md, " &#124;")
			}
		})
	})
}

func (s *SpecMarkdownTestSuite) TestSlashes(t *gotest.T) {
	t.When("an It's label holds a slash", func(w *gotest.T) {
		w.It("a run's spec shows it as one It under its When", func(it *gotest.T) {
			gotest.Contains(it, s.live.Stdout, "| **when a path is read** | | |\n| &nbsp;&nbsp;reads a/b paths | PASS | ")
			gotest.NotContains(it, s.live.Stdout, "reads a**")
		})

		w.It("the spec read from source shows it as one It under its When", func(it *gotest.T) {
			gotest.Contains(it, s.static.Stdout, "| **when a path is read** |\n| &nbsp;&nbsp;reads a/b paths |\n")
			gotest.NotContains(it, s.static.Stdout, "reads a**")
		})
	})

	t.When("an It's label is an earlier It's label, a slash and more", func(w *gotest.T) {
		w.It("a run's spec shows both as Its under their When", func(it *gotest.T) {
			gotest.Contains(it, s.live.Stdout, "| **when a path is read after its parent** | | |\n| &nbsp;&nbsp;reads a | PASS | ")
			gotest.Regexp(it, `\| &nbsp;&nbsp;reads a \| PASS \| [^\n]*\n\| &nbsp;&nbsp;reads a/b \| PASS \| `, s.live.Stdout)
		})

		w.It("the spec read from source shows both as Its under their When", func(it *gotest.T) {
			gotest.Contains(it, s.static.Stdout, "| **when a path is read after its parent** |\n| &nbsp;&nbsp;reads a |\n| &nbsp;&nbsp;reads a/b |\n")
		})
	})
}

func (s *SpecMarkdownTestSuite) TestGroupFailure(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "spec", "--format=md", "../testdata/specgroupfail/")
	gotest.NoError(t, err)
	t.When("a group fails with every behavior under it passing", func(w *gotest.T) {
		w.It("shows the failure and what it wrote on the group's row", func(it *gotest.T) {
			gotest.Equal(it, 1, run.Exit, "spec: %s", run.Stderr)
			gotest.Regexp(it, `\| \*\*when the disk \\\| is full\*\* — [^|]*no space &lt;left&gt; \| FAIL \| `, run.Stdout)
			gotest.Contains(it, run.Stdout, "| &nbsp;&nbsp;starts | PASS | ")
		})
	})
}

func (s *SpecMarkdownTestSuite) TestExcluded(t *gotest.T) {
	t.When("the spec is read from source with X_ suites and methods", func(w *gotest.T) {
		w.It("marks each excluded suite, method and behavior EXCLUDED", func(it *gotest.T) {
			for _, want := range []string{
				"## Parked — EXCLUDED\n\n### Anything — EXCLUDED\n\n| Behavior |\n|----------|\n| **when the suite is parked** — EXCLUDED |\n| &nbsp;&nbsp;never runs — EXCLUDED |\n",
				"### Parked — EXCLUDED\n\n| Behavior |\n|----------|\n| **when the method is parked** — EXCLUDED |\n| &nbsp;&nbsp;never runs — EXCLUDED |\n",
				"## Labels\n\n### Markup\n",
			} {
				gotest.Contains(it, s.static.Stdout, want)
			}
		})
	})
}

// SpecSlashesTestSuite: labels with slashes and a suite declared in both test
// packages, read alike by a run, its replayed stream, the source, the declared
// spec and discover.
type SpecSlashesTestSuite struct {
	live, replay, static, declared specDoc
	stream                         string
	discover                       discoverDoc
}

const slashesFixture = "../testdata/specslashes/"

// SuiteConfig: the live spec and the captured stream run the fixture through
// the whole pipeline.
func (s *SpecSlashesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SpecSlashesTestSuite) BeforeAll(t *gotest.T) {
	s.live = specJSON(t, 0, slashesFixture)
	stream, err := testkit.RunCLI(t.Context(), ".", "-json", slashesFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, stream.Exit, "-json: %s", stream.Stderr)
	s.stream = stream.Stdout
	events := filepath.Join(t.TempDir(), "events.json")
	gotest.NoError(t, os.WriteFile(events, []byte(stream.Stdout), 0o600))
	s.replay = specJSON(t, 0, "--input="+events)
	s.static = specJSON(t, 0, "--static", slashesFixture)
	s.declared = specJSON(t, 0, "--input="+events, "--declared", slashesFixture)
	disc, err := testkit.RunCLI(t.Context(), ".", "discover", slashesFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, disc.Exit, "discover: %s", disc.Stderr)
	gotest.NoError(t, json.Unmarshal([]byte(disc.Stdout), &s.discover), "discover: %s", disc.Stdout)
}

func (s *SpecSlashesTestSuite) AfterAll(t *gotest.T) {}

type discoverDoc struct {
	Packages []struct {
		Suites []discoverSuite `json:"suites"`
	} `json:"packages"`
}

type discoverSuite struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
	Methods  []struct {
		Name      string             `json:"name"`
		Behaviors []discoverBehavior `json:"behaviors"`
	} `json:"methods"`
}

type discoverBehavior struct {
	Name     string             `json:"name"`
	Kind     string             `json:"kind"`
	Children []discoverBehavior `json:"children"`
}

// leaves is the name of each node, failing t unless every node has no children
// and the given status.
func leaves(t *gotest.T, nodes []specNode, status string) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		gotest.Empty(t, n.Children, "%q has children", n.Name)
		gotest.Equal(t, status, n.Status, "%q", n.Name)
		out = append(out, n.Name)
	}
	return out
}

// its is the leaves under the When of SlashesTestSuite's method.
func its(t *gotest.T, doc specDoc, status, method, when string) []string {
	gotest.Len(t, doc.Packages, 1)
	return leaves(t, node(t, doc.Packages[0].Nodes, "TestSlashesTestSuite", method, when).Children, status)
}

// discovered is the behaviors discover reports under the When of the internal
// suite's method, failing t unless each is an It without children.
func (s *SpecSlashesTestSuite) discovered(t *gotest.T, suite, method, when string) []string {
	gotest.Len(t, s.discover.Packages, 1)
	var out []string
	for _, su := range s.discover.Packages[0].Suites {
		if su.Name != suite || su.External {
			continue
		}
		for _, m := range su.Methods {
			if m.Name != method {
				continue
			}
			for _, w := range m.Behaviors {
				if w.Name != when {
					continue
				}
				for _, b := range w.Children {
					gotest.Equal(t, "it", b.Kind, "%q", b.Name)
					gotest.Empty(t, b.Children, "%q has children", b.Name)
					out = append(out, b.Name)
				}
			}
		}
	}
	return out
}

var (
	absolute    = []string{"/abs/pkg", "/usr/local/go/src", "https://_URI", "C:/go/bin", `C:\go\bin`}
	directories = []string{"gets_./", "lists_pkg/"}
)

func (s *SpecSlashesTestSuite) TestLeadingSlash(t *gotest.T) {
	t.When("an It's label starts with a slash", func(w *gotest.T) {
		w.It("a run's spec shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, absolute, its(it, s.live, "pass", "TestLabels", "an_address_is_absolute"))
		})
		w.It("the spec read from source shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, absolute, its(it, s.static, "none", "TestLabels", "an_address_is_absolute"))
		})
		w.It("the spec replayed from the run's -json stream shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, absolute, its(it, s.replay, "pass", "TestLabels", "an_address_is_absolute"))
		})
		w.It("discover reports it as one behavior", func(it *gotest.T) {
			gotest.Equal(it, absolute, s.discovered(it, "SlashesTestSuite", "TestLabels", "an_address_is_absolute"))
		})
	})
}

func (s *SpecSlashesTestSuite) TestTrailingSlash(t *gotest.T) {
	t.When("an It's label ends with a slash", func(w *gotest.T) {
		w.It("a run's spec shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, directories, its(it, s.live, "pass", "TestLabels", "a_directory_is_named"))
		})
		w.It("the spec read from source shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, directories, its(it, s.static, "none", "TestLabels", "a_directory_is_named"))
		})
		w.It("the spec replayed from the run's -json stream shows it as one It under its When", func(it *gotest.T) {
			gotest.Equal(it, directories, its(it, s.replay, "pass", "TestLabels", "a_directory_is_named"))
		})
		w.It("discover reports it as one behavior", func(it *gotest.T) {
			gotest.Equal(it, directories, s.discovered(it, "SlashesTestSuite", "TestLabels", "a_directory_is_named"))
		})
	})
}

// whens is the names of the Whens under SlashesTestSuite's TestLabels.
func whens(t *gotest.T, doc specDoc) []string {
	var out []string
	for _, n := range node(t, doc.Packages[0].Nodes, "TestSlashesTestSuite", "TestLabels").Children {
		out = append(out, n.Name)
	}
	return out
}

func (s *SpecSlashesTestSuite) TestWhenWithASlash(t *gotest.T) {
	want := []string{"holds_clauses", "holds_scenarios"}
	t.When("a When's label ends with a slash and holds Its", func(w *gotest.T) {
		w.It("a run's spec shows its Its under it", func(it *gotest.T) {
			gotest.Equal(it, []string{"an_address_is_absolute", "a_directory_is_named", "reads_specs/"}, whens(it, s.live))
			gotest.Equal(it, want, its(it, s.live, "pass", "TestLabels", "reads_specs/"))
		})
		w.It("the spec read from source shows its Its under it", func(it *gotest.T) {
			gotest.Equal(it, []string{"an_address_is_absolute", "a_directory_is_named", "reads_specs/"}, whens(it, s.static))
			gotest.Equal(it, want, its(it, s.static, "none", "TestLabels", "reads_specs/"))
		})
	})
}

func (s *SpecSlashesTestSuite) TestParallelSibling(t *gotest.T) {
	t.When("an It's label extends a parallel sibling's label", func(w *gotest.T) {
		w.It("a run's spec shows both as Its under their When", func(it *gotest.T) {
			// "." pauses before "./pkg" starts: the stream must say so, or
			// the run proves nothing about parallel siblings.
			gotest.Regexp(it, `"Action":"pause",[^\n]*"Test":"TestParallelSlashesTestSuite/TestPatterns/a_package_pattern_is_read/\."[\s\S]*"Action":"run",[^\n]*"Test":"TestParallelSlashesTestSuite/TestPatterns/a_package_pattern_is_read/\./pkg"`, s.stream)
			got := node(it, s.live.Packages[0].Nodes, "TestParallelSlashesTestSuite", "TestPatterns", "a_package_pattern_is_read").Children
			gotest.Equal(it, []string{".", "./pkg"}, leaves(it, got, "pass"))
		})
	})
}

// twins is each TwinTestSuite node of doc, flattened to one line per node
// below it: its path and status, behind internal or external.
func twins(t *gotest.T, doc specDoc) []string {
	var out []string
	var walk func(prefix string, nodes []specNode)
	walk = func(prefix string, nodes []specNode) {
		for _, n := range nodes {
			out = append(out, prefix+n.Name+" "+n.Status)
			walk(prefix+n.Name+"/", n.Children)
		}
	}
	gotest.Len(t, doc.Packages, 1)
	for _, n := range doc.Packages[0].Nodes {
		if n.Name != "TestTwinTestSuite" {
			continue
		}
		if n.External {
			gotest.Equal(t, 2, n.Variant, "the external suite's variant")
			walk("external: ", n.Children)
		} else {
			gotest.Zero(t, n.Variant, "the internal suite's variant")
			walk("internal: ", n.Children)
		}
	}
	return out
}

// twinsRun is what a run of the fixture shows of its twin suites.
var twinsRun = []string{
	"internal: TestShared pass",
	"internal: TestShared/the_twin_is_internal pass",
	"internal: TestShared/the_twin_is_internal/reads_its_own_state pass",
	"internal: TestInternalOnly pass",
	"external: TestShared pass",
	"external: TestShared/the_twin_is_external pass",
	"external: TestShared/the_twin_is_external/it_nests pass",
	"external: TestShared/the_twin_is_external/it_nests/reads_the_exported_API pass",
	"external: TestExternalOnly pass",
}

// suiteFlags is the External flag of each TwinTestSuite node, in order.
func suiteFlags(doc specDoc) []bool {
	var out []bool
	for _, n := range doc.Packages[0].Nodes {
		if n.Name == "TestTwinTestSuite" {
			out = append(out, n.External)
		}
	}
	return out
}

func (s *SpecSlashesTestSuite) TestSuitesOfBothTestPackages(t *gotest.T) {
	t.When("a suite is declared in both test packages", func(w *gotest.T) {
		w.It("a run's spec shows two suites, the external one marked", func(it *gotest.T) {
			gotest.Equal(it, []bool{false, true}, suiteFlags(s.live))
		})
		w.It("each shows only its own methods", func(it *gotest.T) {
			gotest.Equal(it, twinsRun, twins(it, s.live))
		})
		w.It("the spec replayed from the run's -json stream shows the same two suites", func(it *gotest.T) {
			gotest.Equal(it, []bool{false, true}, suiteFlags(s.replay))
			gotest.Equal(it, twinsRun, twins(it, s.replay))
		})
		w.It("the declared spec under the run shows every behavior with its verdict", func(it *gotest.T) {
			gotest.Equal(it, []bool{false, true}, suiteFlags(s.declared))
			gotest.Equal(it, twinsRun, twins(it, s.declared))
			gotest.Zero(it, s.declared.Stats.NotRun, "behaviors the declared spec lost from the run")
			gotest.Equal(it, s.declared.Stats.Behaviors, s.declared.Stats.Passed)
		})
		w.It("discover marks the external suite", func(it *gotest.T) {
			var external []bool
			for _, su := range s.discover.Packages[0].Suites {
				if su.Name == "TwinTestSuite" {
					external = append(external, su.External)
				}
			}
			gotest.Equal(it, []bool{false, true}, external)
		})
	})

	t.When("go test runs a suite of both test packages", func(w *gotest.T) {
		w.It("the internal suite runs first", func(it *gotest.T) {
			// The first suite the run reports holds a method only the
			// internal twin declares, and its events come first.
			gotest.Contains(it, twins(it, s.live), "internal: TestInternalOnly pass")
			internal := strings.Index(s.stream, `"Test":"TestTwinTestSuite/TestInternalOnly"`)
			external := strings.Index(s.stream, `"Test":"TestTwinTestSuite/TestExternalOnly"`)
			gotest.NotEqual(it, -1, internal)
			gotest.NotEqual(it, -1, external)
			gotest.Less(it, internal, external)
		})
	})
}
