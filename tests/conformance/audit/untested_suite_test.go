package audit_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditUntestedTestSuite: a changed package the audit's run executed no
// test of is not proven, unless it changes no line of a function.
type AuditUntestedTestSuite struct {
	dir, cache     string
	untested, docs testkit.Run
}

func (s *AuditUntestedTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *AuditUntestedTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-untested-")
	gotest.NoError(t, err)
	s.dir = dir
	s.cache = filepath.Join(dir, "cache")
	s.untested = auditIn(t, gatesCheckout(t, dir, "untested", "untested"), s.cache, "--base=HEAD", "--run", "--fail-on=unproven", "--format=json")
	s.docs = auditIn(t, gatesCheckout(t, dir, "docs", "docs"), s.cache, "--base=HEAD", "--run", "--fail-on=unproven", "--format=json")
}

func (s *AuditUntestedTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditUntestedTestSuite) TestWhatTheRunTestedNothingOf(t *gotest.T) {
	t.When("audit --run compares a head that adds a package with a function and no test", func(t *gotest.T) {
		t.It("lists the package's file as without coverage data and --fail-on=unproven exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.untested.Exit, "stderr: %s", s.untested.Stderr)
			doc := readRunDoc(it, s.untested.Stdout)
			gotest.Len(it, doc.Unproven, 1)
			u := doc.Unproven[0]
			u.Ranges = nil
			gotest.Equal(it, unproven{File: "pay/pay.go"}, u)
			gotest.NotZero(it, doc.Coverage)
			gotest.Empty(it, doc.Coverage.Packages)
			gotest.Equal(it, 1, doc.Summary.Classes["unproven"])
		})
	})
	t.When("audit --run compares a head that adds a package declaring only a type", func(t *gotest.T) {
		t.It("lists no unproven file and --fail-on=unproven exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.docs.Exit, "stderr: %s", s.docs.Stderr)
			doc := readRunDoc(it, s.docs.Stdout)
			gotest.Empty(it, doc.Unproven)
			gotest.Zero(it, doc.Coverage)
			gotest.Len(it, doc.Streams, 1)
		})
	})
}
