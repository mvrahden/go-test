package spec_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecLongLineTestSuite: a saved stream with an event line longer than any
// line buffer still renders.
type SpecLongLineTestSuite struct{}

func (s *SpecLongLineTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 30 * time.Second}
}

func (s *SpecLongLineTestSuite) TestALineLongerThanAMebibyte(t *gotest.T) {
	t.When("a saved stream holds an event line longer than a mebibyte", func(w *gotest.T) {
		path := filepath.Join(w.TempDir(), "long.jsonl")
		stream := strings.Join([]string{
			`{"Action":"run","Package":"p","Test":"TestLong"}`,
			`{"Action":"output","Package":"p","Test":"TestLong","Output":"    long_test.go:3: ` + strings.Repeat("x", 2<<20) + `\n"}`,
			`{"Action":"fail","Package":"p","Test":"TestLong","Elapsed":0}`,
			`{"Action":"fail","Package":"p","Elapsed":0}`,
		}, "\n") + "\n"
		gotest.NoError(w, os.WriteFile(path, []byte(stream), 0o600))

		w.It("spec renders the stream, the long line's test with its verdict", func(it *gotest.T) {
			doc := specJSON(it, 1, "--input="+path)
			gotest.NotEmpty(it, doc.Packages)
			gotest.Equal(it, "fail", node(it, doc.Packages[0].Nodes, "TestLong").Status)
		})
		w.It("summary renders it and exits by its verdict", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", "--input="+path)
			gotest.NoError(it, err)
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^1 test failed\n`, run.Stdout)
		})
	})
}
