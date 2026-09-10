package gotestspec_test

import (
	"bytes"
	"encoding/json"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzNodeTestSuite pins how a generated Fuzz<Suite>_<Method> wrapper lands
// in the spec: under its suite, as a fuzz target whose seed replays are its
// evidence — never as a stdlib test beside the suite.
type FuzzNodeTestSuite struct{}

func (s *FuzzNodeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func ev(action gotestspec.Action, test string) gotestspec.TestEvent {
	return gotestspec.TestEvent{Action: action, Package: "example.com/codec", Test: test}
}

// fuzzStream is what go test -json emits for a suite and one fuzz wrapper
// replaying two seeds.
func fuzzStream() []gotestspec.TestEvent {
	return []gotestspec.TestEvent{
		ev(gotestspec.ActionRun, "TestFrameCodecTestSuite"),
		ev(gotestspec.ActionRun, "TestFrameCodecTestSuite/TestEncode"),
		ev(gotestspec.ActionPass, "TestFrameCodecTestSuite/TestEncode"),
		ev(gotestspec.ActionPass, "TestFrameCodecTestSuite"),
		ev(gotestspec.ActionRun, "FuzzFrameCodecTestSuite_FuzzRoundTrip"),
		ev(gotestspec.ActionRun, "FuzzFrameCodecTestSuite_FuzzRoundTrip/seed#0"),
		ev(gotestspec.ActionPass, "FuzzFrameCodecTestSuite_FuzzRoundTrip/seed#0"),
		ev(gotestspec.ActionRun, "FuzzFrameCodecTestSuite_FuzzRoundTrip/seed#1"),
		ev(gotestspec.ActionPass, "FuzzFrameCodecTestSuite_FuzzRoundTrip/seed#1"),
		ev(gotestspec.ActionPass, "FuzzFrameCodecTestSuite_FuzzRoundTrip"),
	}
}

func fuzzNodeOf(t *gotest.T, pkgs []*gotestspec.Package) (*gotestspec.Node, *gotestspec.Node) {
	gotest.Len(t, pkgs, 1)
	gotest.Len(t, pkgs[0].Nodes, 1, "the wrapper must not stay a top-level node")
	suite := pkgs[0].Nodes[0]
	for _, c := range suite.Children {
		if c.Kind == gotestspec.KindFuzz {
			return suite, c
		}
	}
	gotest.True(t, false, "no fuzz child under %s: %v", suite.Name, suite.Children)
	return suite, nil
}

func (s *FuzzNodeTestSuite) TestTree(t *gotest.T) {
	t.When("the suite and its fuzz wrapper both ran", func(w *gotest.T) {
		suite, fz := fuzzNodeOf(w, gotestspec.BuildTree(fuzzStream()))

		w.It("hangs the target under the suite by its method name", func(it *gotest.T) {
			gotest.Equal(it, gotestspec.KindSuite, suite.Kind)
			gotest.Equal(it, "FrameCodec", suite.Display)
			gotest.Equal(it, "FuzzRoundTrip", fz.Name)
			gotest.Equal(it, "RoundTrip", fz.Display)
			gotest.Equal(it, gotestspec.StatusPass, fz.Status)
		})
		w.It("keeps the seed replays numbered instead of stripping them as duplicates", func(it *gotest.T) {
			gotest.Len(it, fz.Children, 2)
			gotest.Equal(it, "seed #1", fz.Children[0].Display)
			gotest.Equal(it, "seed #2", fz.Children[1].Display)
			gotest.Equal(it, gotestspec.KindBlock, fz.Children[0].Kind)
		})
	})

	t.When("a run filter left the suite out of the stream", func(w *gotest.T) {
		events := fuzzStream()[4:]
		suite, fz := fuzzNodeOf(w, gotestspec.BuildTree(events))

		w.It("synthesizes the suite so the target still has a home", func(it *gotest.T) {
			gotest.Equal(it, "TestFrameCodecTestSuite", suite.Name)
			gotest.Equal(it, "FrameCodec", suite.Display)
			gotest.Equal(it, gotestspec.StatusPass, suite.Status)
			gotest.Equal(it, "RoundTrip", fz.Display)
		})
	})

	t.When("a committed corpus entry fails on replay", func(w *gotest.T) {
		events := fuzzStream()[:9]
		events = append(events,
			ev(gotestspec.ActionRun, "FuzzFrameCodecTestSuite_FuzzRoundTrip/f5000abc"),
			gotestspec.TestEvent{Action: gotestspec.ActionOutput, Package: "example.com/codec", Test: "FuzzFrameCodecTestSuite_FuzzRoundTrip/f5000abc", Output: "    codec_test.go:12: boom\n"},
			ev(gotestspec.ActionFail, "FuzzFrameCodecTestSuite_FuzzRoundTrip/f5000abc"),
			ev(gotestspec.ActionFail, "FuzzFrameCodecTestSuite_FuzzRoundTrip"),
		)
		pkgs := gotestspec.BuildTree(events)
		suite, fz := fuzzNodeOf(w, pkgs)

		w.It("names the entry and fails the target", func(it *gotest.T) {
			gotest.Equal(it, gotestspec.StatusFail, fz.Status)
			gotest.Equal(it, "f5000abc", fz.Children[2].Display)
			gotest.Equal(it, gotestspec.StatusFail, fz.Children[2].Status)
		})
		w.It("reports the failure under the suite and target", func(it *gotest.T) {
			var buf bytes.Buffer
			gotestspec.RenderSummary(&buf, pkgs, gotestspec.WithNoColor())
			gotest.Contains(it, buf.String(), suite.Display+" / RoundTrip / f5000abc")
			gotest.Contains(it, buf.String(), "codec_test.go:12: boom")
		})
	})
}

func (s *FuzzNodeTestSuite) TestStats(t *gotest.T) {
	pkgs := gotestspec.BuildTree(fuzzStream())
	stats := gotestspec.CollectStats(pkgs)

	t.It("counts the target once, as a fuzz target, never its seeds as behaviors", func(it *gotest.T) {
		gotest.Equal(it, 1, stats.Fuzzers)
		gotest.Equal(it, 1, stats.Behaviors)
		gotest.Equal(it, 0, stats.Tests)
		gotest.Equal(it, 2, stats.Passed)
	})

	t.It("carries the count and the kind into JSON", func(it *gotest.T) {
		var buf bytes.Buffer
		gotestspec.RenderJSON(&buf, pkgs)
		var doc struct {
			Packages []struct {
				Nodes []struct {
					Children []struct {
						Kind     string `json:"kind"`
						Name     string `json:"name"`
						Children []struct {
							Display string `json:"display"`
						} `json:"children"`
					} `json:"children"`
				} `json:"nodes"`
			} `json:"packages"`
			Stats struct {
				Fuzzers int `json:"fuzzers"`
			} `json:"stats"`
		}
		gotest.NoError(it, json.Unmarshal(buf.Bytes(), &doc))
		gotest.Equal(it, 1, doc.Stats.Fuzzers)
		kinds := map[string]string{}
		for _, c := range doc.Packages[0].Nodes[0].Children {
			kinds[c.Name] = c.Kind
		}
		gotest.Equal(it, "fuzz", kinds["FuzzRoundTrip"])
	})
}

func (s *FuzzNodeTestSuite) TestRendering(t *gotest.T) {
	pkgs := gotestspec.BuildTree(fuzzStream())

	t.It("collapses a passing target to one line with its seed count", func(it *gotest.T) {
		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, pkgs, gotestspec.WithNoColor())
		out := buf.String()
		gotest.Contains(it, out, "RoundTrip  2 seeds")
		gotest.NotContains(it, out, "seed #1")
		gotest.Contains(it, out, "1 fuzz target")
	})

	t.It("lists the seeds when one of them failed", func(it *gotest.T) {
		events := fuzzStream()[:9]
		events = append(events,
			ev(gotestspec.ActionRun, "FuzzFrameCodecTestSuite_FuzzRoundTrip/f5000abc"),
			ev(gotestspec.ActionFail, "FuzzFrameCodecTestSuite_FuzzRoundTrip/f5000abc"),
			ev(gotestspec.ActionFail, "FuzzFrameCodecTestSuite_FuzzRoundTrip"),
		)
		var buf bytes.Buffer
		gotestspec.RenderTerminal(&buf, gotestspec.BuildTree(events), gotestspec.WithNoColor())
		out := buf.String()
		gotest.Contains(it, out, "seed #1")
		gotest.Contains(it, out, "f5000abc")
	})

	t.It("counts fuzz targets in the markdown trailer", func(it *gotest.T) {
		var buf bytes.Buffer
		gotestspec.RenderMarkdown(&buf, pkgs)
		gotest.Contains(it, buf.String(), "1 fuzz target")
		gotest.Contains(it, buf.String(), "RoundTrip")
	})
}
