package main_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecInputTestSuite covers where 'gotest spec' takes its events from — stdin, a file or a live run — and the one exit rule they share.
type SpecInputTestSuite struct{}

func (s *SpecInputTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// specInputCtx holds the overlay dir a live run of the examples writes.
type specInputCtx struct{ overlayDir string }

func (s *SpecInputTestSuite) BeforeEach(_ *gotest.T) *specInputCtx { return &specInputCtx{} }

func (s *SpecInputTestSuite) AfterEach(t *gotest.T, ctx *specInputCtx) {
	if ctx.overlayDir != "" {
		gotest.NoError(t, os.RemoveAll(ctx.overlayDir))
	}
}

func (s *SpecInputTestSuite) TestRunSpec_InputStdin(t *gotest.T, ctx *specInputCtx) {
	t.It("renders spec output from stdin-like JSON", func(it *gotest.T) {
		absExamples, err := filepath.Abs(filepath.Join("..", "..", "examples"))
		gotest.NoError(it, err, "%v", err)
		if _, err := os.Stat(filepath.Join(absExamples, "go.mod")); err != nil {
			it.Skipf("examples directory not found: %v", err)
		}

		loaded, _, err := gotestgen.LoadPackages([]string{filepath.Join(absExamples, "cart")}, nil)
		gotest.NoError(it, err, "LoadPackages: %v", err)
		results, _, err := gotestgen.GenerateFromLoaded(loaded)
		gotest.NoError(it, err, "GenerateFromLoaded: %v", err)

		tmpDir, err := gotestrunner.WriteOverlay(results)
		gotest.NoError(it, err, "WriteOverlay: %v", err)
		ctx.overlayDir = tmpDir

		jsonData, _, err := gotestrunner.StdlibRunTestsJSONIn(context.Background(), absExamples,
			[]string{"-overlay=" + filepath.Join(tmpDir, "overlay.json"), "./cart"})
		gotest.NoError(it, err, "go test: %v", err)

		events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
		gotest.NoError(it, err, "ParseEvents: %v", err)

		tree := gotestspec.BuildTree(events)

		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, tree, gotestspec.WithNoColor())

		output := buf.String()
		gotest.Contains(it, output, "ShoppingCart")
	})
}

func (s *SpecInputTestSuite) TestInputModesShareOneExitRule(t *gotest.T, _ *specInputCtx) {
	// One failing and one passing stream, saved the way CI replays them. In a
	// pipe without pipefail the exit code of the rendering command is the only
	// verdict CI ever sees — and `spec --input` used to return 0 on anything.
	failingStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"output","Package":"example.com/pkg","Test":"TestBoom","Output":"--- FAIL: TestBoom (0.00s)\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"fail","Package":"example.com/pkg"}
`
	greenStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestOK"}
{"Action":"pass","Package":"example.com/pkg"}
`

	writeStream := func(w *gotest.T, content string) string {
		path := filepath.Join(w.TempDir(), "events.json")
		gotest.NoError(w, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	t.When("the stream contains a failure", func(w *gotest.T) {
		input := writeStream(w, failingStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("spec --input exits nonzero", func(it *gotest.T) {
			gotest.Equal(it, 1, ExportRunSpecFromInput(input, "terminal", out, true, false))
		})

		w.It("summary --input agrees", func(it *gotest.T) {
			gotest.Equal(it, 1, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, false, ""))
		})
	})

	t.When("the stream is clean", func(w *gotest.T) {
		input := writeStream(w, greenStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("both exit zero", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, false))
			gotest.Equal(it, 0, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, false, ""))
		})
	})
}

// TestRenderOnlySeparatesVerdictFromRendering covers --render-only: the exit
// code drops the test verdict but still reports a failure to render.
func (s *SpecInputTestSuite) TestRenderOnlySeparatesVerdictFromRendering(t *gotest.T, _ *specInputCtx) {
	failingStream := `{"Action":"run","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"output","Package":"example.com/pkg","Test":"TestBoom","Output":"--- FAIL: TestBoom (0.00s)\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestBoom"}
{"Action":"fail","Package":"example.com/pkg"}
`

	writeStream := func(w *gotest.T, content string) string {
		path := filepath.Join(w.TempDir(), "events.json")
		gotest.NoError(w, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	t.When("a failing stream is rendered with --render-only", func(w *gotest.T) {
		input := writeStream(w, failingStream)
		out := filepath.Join(w.TempDir(), "out.txt")

		w.It("spec reports success, because rendering succeeded", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, true))
		})

		w.It("summary agrees, keeping the two input modes on one rule", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, true, ""))
		})

		w.It("still renders the failure into the output", func(it *gotest.T) {
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "terminal", out, true, true))
			data, err := os.ReadFile(out)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "Boom")
		})
	})

	t.When("the input cannot be read", func(w *gotest.T) {
		missing := filepath.Join(w.TempDir(), "absent.json")
		out := filepath.Join(w.TempDir(), "out.txt")

		// The whole point of the flag is to suppress a verdict about the tests,
		// never a report that the command itself failed.
		w.It("spec still fails, because nothing was rendered", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSpecFromInput(missing, "terminal", out, true, true))
		})

		w.It("summary still fails too", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSummaryFromInput(missing, "terminal", out, "", true, false, true, ""))
		})
	})

	t.When("--render-only is passed without --input", func(w *gotest.T) {
		// Suppressing the verdict of a real run would turn a red pipeline green,
		// so the flag is refused outside the replay path it was built for.
		w.It("spec rejects it as a usage error", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSpec(Invocation{Args: []string{"--render-only", "./..."}}))
		})

		w.It("summary rejects it as a usage error", func(it *gotest.T) {
			gotest.Equal(it, 2, ExportRunSummary(Invocation{Args: []string{"--render-only", "./..."}}))
		})
	})
}

// TestInputReadsTheSourceItCanReach covers replay labels: read from source when
// the stream's packages are reachable, from subtest names otherwise.
func (s *SpecInputTestSuite) TestInputReadsTheSourceItCanReach(t *gotest.T, _ *specInputCtx) {
	const pkg = "github.com/mvrahden/go-test/examples/search"
	const when = "TestArticleSearchTestSuite/TestSearchByTitle/searching_for_a_title_keyword"
	stream := `{"Action":"run","Package":"` + pkg + `","Test":"TestArticleSearchTestSuite"}
{"Action":"run","Package":"` + pkg + `","Test":"TestArticleSearchTestSuite/TestSearchByTitle"}
{"Action":"run","Package":"` + pkg + `","Test":"` + when + `"}
{"Action":"run","Package":"` + pkg + `","Test":"` + when + `/finds_the_matching_article"}
{"Action":"output","Package":"` + pkg + `","Test":"` + when + `/finds_the_matching_article","Output":"    suite_test.go:31: Len failed\n"}
{"Action":"fail","Package":"` + pkg + `","Test":"` + when + `/finds_the_matching_article"}
{"Action":"fail","Package":"` + pkg + `","Test":"` + when + `"}
{"Action":"fail","Package":"` + pkg + `","Test":"TestArticleSearchTestSuite/TestSearchByTitle"}
{"Action":"fail","Package":"` + pkg + `","Test":"TestArticleSearchTestSuite"}
{"Action":"fail","Package":"` + pkg + `"}
`
	input := filepath.Join(t.TempDir(), "events.json")
	gotest.NoError(t, os.WriteFile(input, []byte(stream), 0o600))

	t.When("the stream's packages are in the module the command runs in", func(w *gotest.T) {
		w.It("spec renders the declared label in its vocabulary", func(it *gotest.T) {
			out := filepath.Join(it.TempDir(), "spec.json")
			gotest.Equal(it, 0, ExportRunSpecFromInput(input, "json", out, true, true))
			data, err := os.ReadFile(out)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), `"display":"when searching for a title keyword"`)
			gotest.Contains(it, string(data), `"vocab":"when"`)
		})

		w.It("summary titles the failure the same way", func(it *gotest.T) {
			out := filepath.Join(it.TempDir(), "summary.txt")
			gotest.Equal(it, 0, ExportRunSummaryFromInput(input, "terminal", out, "", true, false, true, ""))
			data, err := os.ReadFile(out)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "when searching for a title keyword")
		})
	})

	t.When("the stream's packages cannot be found", func(w *gotest.T) {
		foreign := filepath.Join(w.TempDir(), "events.json")
		gotest.NoError(w, os.WriteFile(foreign, []byte(strings.ReplaceAll(stream, pkg, "example.com/elsewhere")), 0o600))

		w.It("spec still renders, from the names alone", func(it *gotest.T) {
			out := filepath.Join(it.TempDir(), "spec.json")
			gotest.Equal(it, 0, ExportRunSpecFromInput(foreign, "json", out, true, true))
			data, err := os.ReadFile(out)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), `"display":"searching for a title keyword"`)
		})
	})
}
