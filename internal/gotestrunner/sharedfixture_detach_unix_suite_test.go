//go:build !windows

package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureDetachedGrandchildTestSuite drives a shared fixture process whose
// fixture leaves a session-leader child on the protocol pipe. Exclusive: it
// takes a wall-clock verdict on the teardown. Sequential: Setenv.
type SharedFixtureDetachedGrandchildTestSuite struct {
	stopFile string
}

func (s *SharedFixtureDetachedGrandchildTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *SharedFixtureDetachedGrandchildTestSuite) BeforeEach(t *gotest.T) {
	s.stopFile = filepath.Join(t.TempDir(), "stop")
	t.Setenv(fixtures.EnvDetachingStop, s.stopFile)
}

// AfterEach releases the child, which polls for the stop file.
func (s *SharedFixtureDetachedGrandchildTestSuite) AfterEach(t *gotest.T) {
	_ = os.WriteFile(s.stopFile, []byte("stop"), 0o600)
}

func detachingFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "DetachingSharedFixture",
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.DetachingSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

// The fixture process exits as soon as its AfterAll is done; only the child it
// left holds the protocol pipe. The teardown verdict is the exit status, and
// it arrives once the drain delay passes.
func (s *SharedFixtureDetachedGrandchildTestSuite) TestTeardownDoesNotWaitForTheGrandchild(t *gotest.T) {
	const budget = 10 * time.Second
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), detachingFixture(), budget)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitAllReady(ctx, budget), "waiting for setup")
	cancel()

	start := time.Now()
	err = proc.Teardown()
	elapsed := time.Since(start)

	t.It("reports the teardown that finished", func(it *gotest.T) {
		gotest.NoError(it, err)
	})
	t.It("returns once the drain delay passes, not when the grandchild lets go", func(it *gotest.T) {
		gotest.Less(it, elapsed, gotestrunner.OutputDrainDelay+3*time.Second, "Teardown blocked %v on a pipe a detached grandchild holds", elapsed)
	})
}
