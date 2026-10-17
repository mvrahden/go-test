package main_test

import (
	"bytes"
	"strings"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// WatchTestSuite covers the watch subcommand's file filter and pattern derivation.
type WatchTestSuite struct{}

func (s *WatchTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *WatchTestSuite) TestWatchHelpers(t *gotest.T) {
	t.When("IsGoFile", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc   string
			name   string
			expect bool
		}{
			{Desc: "go file", name: "main.go", expect: true},
			{Desc: "test file", name: "main_test.go", expect: true},
			{Desc: "path with go file", name: "/tmp/foo/bar.go", expect: true},
			{Desc: "not a go file", name: "main.py", expect: false},
			{Desc: "go in middle", name: "foo.go.bak", expect: false},
			{Desc: "empty", name: "", expect: false},
		}) {
			gotest.Equal(sub, tc.expect, ExportIsGoFile(tc.name))
		}
	})

	t.When("DirsToPatterns", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc string
			dirs []string
			want []string
		}{
			{Desc: "single dir", dirs: []string{"pkg/foo"}, want: []string{"./pkg/foo"}},
			{Desc: "multiple dirs keep their order", dirs: []string{"pkg/foo", "cmd/bar"}, want: []string{"./pkg/foo", "./cmd/bar"}},
			{Desc: "an absolute dir stays absolute", dirs: []string{"/m/pkg"}, want: []string{"/m/pkg"}},
			{Desc: "empty", dirs: nil, want: []string{}},
		}) {
			gotest.Equal(sub, tc.want, ExportDirsToPatterns(tc.dirs))
		}
	})

	t.When("ReplacePatterns", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct { //nolint:gocritic // rangeValCopy: intentional
			Desc        string
			original    []string
			newPatterns []string
			expected    []string
		}{
			{
				Desc:        "replaces package pattern",
				original:    []string{"-v", "./pkg/foo", "-race"},
				newPatterns: []string{"./cmd/bar"},
				expected:    []string{"-v", "-race", "./cmd/bar"},
			},
			{
				Desc:        "no patterns to replace",
				original:    []string{"-v", "-race"},
				newPatterns: []string{"./pkg/new"},
				expected:    []string{"-v", "-race", "./pkg/new"},
			},
			{
				Desc:        "keeps flag values and replaces import paths",
				original:    []string{"-run", "X", "fmt", "-count", "2"},
				newPatterns: []string{"./changed"},
				expected:    []string{"-run", "X", "-count", "2", "./changed"},
			},
			{
				Desc:        "multiple patterns replaced",
				original:    []string{"-v", "./pkg/a", "./pkg/b", "-race"},
				newPatterns: []string{"./changed"},
				expected:    []string{"-v", "-race", "./changed"},
			},
		}) {
			result := ExportReplacePatterns(tc.original, tc.newPatterns)
			gotest.Equal(sub, tc.expected, result)
		}
	})
}

func (s *WatchTestSuite) TestRenderWatchRun(t *gotest.T) {
	stream := []byte(`{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse"}
{"Action":"output","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite/BenchmarkParse","Output":"BenchmarkFooTestSuite/BenchmarkParse-8   \t 1201 \t 985.2 ns/op\n"}
{"Action":"pass","Package":"example.com/pkg","Test":"BenchmarkFooTestSuite","Elapsed":0.01}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.02}
`)

	t.When("a bench iteration follows an earlier one", func(w *gotest.T) {
		first, err := ExportRenderWatchRun(&bytes.Buffer{}, stream, false, false, true, nil, nil)
		gotest.NoError(w, err)
		prev := map[string]float64{}
		for key, ns := range first {
			prev[key] = ns / 2
		}
		var out bytes.Buffer
		_, err = ExportRenderWatchRun(&out, stream, false, false, true, nil, prev)

		w.It("draws it once, with the delta against the previous iteration", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, 1, strings.Count(out.String(), "(Δ "), out.String())
			gotest.Contains(it, out.String(), "(Δ +100.0%)")
		})
	})

	t.When("the editor watches a bench run as JSON", func(w *gotest.T) {
		var out bytes.Buffer
		_, err := ExportRenderWatchRun(&out, stream, true, false, true, nil, nil)

		w.It("passes the captured stream through once", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, string(stream), out.String())
		})
	})
}
