package gotestrunner_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzSessionTestSuite: a session records the crasher files it wrote, and
// only those, with their content.
type FuzzSessionTestSuite struct{}

func (s *FuzzSessionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func plantCrasher(t *gotest.T, dir, fn, name, body string) {
	d := filepath.Join(dir, "testdata", "fuzz", fn)
	gotest.NoError(t, os.MkdirAll(d, 0o750))
	gotest.NoError(t, os.WriteFile(filepath.Join(d, name), []byte(body), 0o600))
}

func (s *FuzzSessionTestSuite) TestRecord(t *gotest.T) {
	record := gotestrunner.FuzzSessionFile(t.TempDir())
	dir := t.TempDir()
	plantCrasher(t, dir, "FuzzA", "new", "go test fuzz v1\nstring(\"a\")\n")
	plantCrasher(t, dir, "FuzzA", "old", "go test fuzz v1\nstring(\"b\")\n")
	targets := []gotestrunner.FuzzTarget{{Package: "p", Dir: dir, Func: "FuzzA"}, {Package: "p", Dir: dir, Func: "FuzzB"}}
	res := gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{{Func: "FuzzA", NewCrashers: []string{"new"}}, {Func: "FuzzB"}}}
	gotest.NoError(t, gotestrunner.RecordFuzzSession(record, targets, res))

	session, ok, err := gotestrunner.ReadFuzzSession(record)
	gotest.NoError(t, err)
	gotest.True(t, ok)

	t.It("lists the files the session wrote under their target", func(it *gotest.T) {
		got := session.For(targets[0])
		gotest.Len(it, got, 1)
		gotest.Equal(it, "new", got[0].Name)
		gotest.NoError(it, got[0].Check())
		gotest.Empty(it, session.For(targets[1]))
	})
	t.It("tells a file changed since the session apart", func(it *gotest.T) {
		c := session.For(targets[0])[0]
		gotest.NoError(it, os.WriteFile(c.Path(), []byte("go test fuzz v1\nstring(\"z\")\n"), 0o600))
		gotest.ErrorContains(it, c.Check(), "changed since the fuzz session wrote it")
	})
	t.It("is replaced whole by the next session", func(it *gotest.T) {
		gotest.NoError(it, gotestrunner.RecordFuzzSession(record, targets, gotestrunner.FuzzRunResult{Outcomes: []gotestrunner.FuzzTargetOutcome{{Func: "FuzzA"}, {Func: "FuzzB"}}}))
		next, ok, err := gotestrunner.ReadFuzzSession(record)
		gotest.NoError(it, err)
		gotest.True(it, ok)
		gotest.Empty(it, next.Crashers)
	})
}

func (s *FuzzSessionTestSuite) TestNoRecord(t *gotest.T) {
	_, ok, err := gotestrunner.ReadFuzzSession(filepath.Join(t.TempDir(), "fuzz-session.json"))
	gotest.NoError(t, err)
	gotest.False(t, ok)
}
