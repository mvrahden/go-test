package specslashes

import "github.com/mvrahden/go-test/pkg/gotest"

type SlashesTestSuite struct{}

func (s *SlashesTestSuite) TestLabels(t *gotest.T) {
	t.When("an address is absolute", func(w *gotest.T) {
		w.It("/abs/pkg", func(it *gotest.T) {})
		w.It("/usr/local/go/src", func(it *gotest.T) {})
		w.It("https:// URI", func(it *gotest.T) {})
		w.It("C:/go/bin", func(it *gotest.T) {})
		w.It(`C:\go\bin`, func(it *gotest.T) {})
	})
	t.When("a directory is named", func(w *gotest.T) {
		w.It("gets ./", func(it *gotest.T) {})
		w.It("lists pkg/", func(it *gotest.T) {})
	})
	t.When("reads specs/", func(w *gotest.T) {
		w.It("holds clauses", func(it *gotest.T) {})
		w.It("holds scenarios", func(it *gotest.T) {})
	})
}

// TestRepeats: names testing makes unique, and one that only looks unique.
func (s *SlashesTestSuite) TestRepeats(t *gotest.T) {
	t.When("a module is listed twice", func(w *gotest.T) {
		w.It("example.com/m/pkg", func(it *gotest.T) {})
		w.It("example.com/m/pkg", func(it *gotest.T) {})
		w.It("seed#01", func(it *gotest.T) {})
	})
	t.When("a path is spelled twice", func(w *gotest.T) {
		w.When("a", func(w *gotest.T) {
			w.It("b", func(it *gotest.T) {})
		})
		w.It("a/b", func(it *gotest.T) {})
	})
}

// ParallelSlashesTestSuite: "." pauses before "./pkg" starts.
type ParallelSlashesTestSuite struct{}

func (s *ParallelSlashesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ParallelSlashesTestSuite) TestPatterns(t *gotest.T) {
	t.When("a package pattern is read", func(w *gotest.T) {
		w.It(".", func(it *gotest.T) { it.T().Parallel() })
		w.It("./pkg", func(it *gotest.T) { it.T().Parallel() })
	})
}

// TwinTestSuite is declared again in the external test package.
type TwinTestSuite struct{}

func (s *TwinTestSuite) TestShared(t *gotest.T) {
	t.When("the twin is internal", func(w *gotest.T) {
		w.It("reads its own state", func(it *gotest.T) {})
	})
}

func (s *TwinTestSuite) TestInternalOnly(t *gotest.T) {
	gotest.Equal(t, "specslashes", Name())
}
