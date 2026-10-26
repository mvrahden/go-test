package gotestrunner_test

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureDAGTestSuite: the setup process starts and releases shared
// fixtures concurrently, each after what it depends on.
// Sequential: the fixtures are configured through the environment.
type SharedFixtureDAGTestSuite struct{}

// SuiteConfig: Exclusive, as every suite that builds and runs a real shared
// fixture subprocess against wall-clock budgets is.
func (s *SharedFixtureDAGTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

const dagPkg = "github.com/mvrahden/go-test/tests/sharedfixture/fixtures"

func dagKey(name string) string { return dagPkg + "." + name + "SharedFixture" }

// dagFixtures describes the named Dag fixtures in dependency order, DagChild
// bound to DagA; deferred ones wait for a start command.
func dagFixtures(deferred bool, names ...string) []gotestgen.SharedFixtureInfo {
	infos := make([]gotestgen.SharedFixtureInfo, 0, len(names))
	for _, name := range names {
		info := gotestgen.SharedFixtureInfo{
			Identifier:    name + "SharedFixture",
			HasAfterAll:   true,
			HasConfig:     true,
			PkgPath:       dagPkg,
			PkgName:       "fixtures",
			QualifiedType: "fixtures." + name + "SharedFixture",
			Deferred:      deferred,
		}
		if name == "DagChild" {
			info.Dependencies = []string{dagKey("DagA")}
			info.DependencyFields = map[string]string{dagKey("DagA"): "Parent"}
		}
		infos = append(infos, info)
	}
	return infos
}

// startDAG boots the setup process for infos and waits for its up-front
// phase. The returned release tears it down and ends its context.
func startDAG(t *gotest.T, infos []gotestgen.SharedFixtureInfo) (proc *gotestrunner.SharedFixtureProcess, release func() error) {
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), infos, 0)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	gotest.NoError(t, proc.WaitAllReady(ctx, 0), "waiting for the up-front phase")
	return proc, func() error {
		defer cancel()
		return proc.Teardown()
	}
}

func (s *SharedFixtureDAGTestSuite) TestALateStartOfIndependentFixturesTakesTheSlowest(t *gotest.T) {
	t.Setenv(fixtures.EnvDagSetupDelay, time.Second.String())
	proc, release := startDAG(t, dagFixtures(true, "DagA", "DagB"))
	began := time.Now()
	err := proc.StartKeys(t.Context(), []string{dagKey("DagA"), dagKey("DagB")}, 0)
	took := time.Since(began)
	teardownErr := release()

	t.When("two independent fixtures taking 1s each start in one late command", func(w *gotest.T) {
		w.It("acknowledges as soon as the slower is up, not after both in turn", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.NoError(it, teardownErr)
			gotest.Less(it, took, 1800*time.Millisecond)
		})
	})
}
