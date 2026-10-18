package gotestrunner_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureProtocolTestSuite: what the shared fixture process writes for
// itself never reaches the runner as protocol, and what reaches the runner as
// protocol is judged, not trusted.
type SharedFixtureProtocolTestSuite struct{}

// SuiteConfig: parallel; the methods share nothing and time nothing.
func (s *SharedFixtureProtocolTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func noisySetupFixture() []gotestgen.SharedFixtureInfo {
	return []gotestgen.SharedFixtureInfo{{
		Identifier:     "NoisySetupSharedFixture",
		HasAfterAll:    true,
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.NoisySetupSharedFixture",
		TransferFields: []string{"Marker"},
	}}
}

func (s *SharedFixtureProtocolTestSuite) TestStdoutOfTheFixtureIsNotProtocol(t *gotest.T) {
	ctx, cancel := context.WithCancel(t.Context())
	proc, err := gotestrunner.StartSharedFixtures(ctx, t.TempDir(), noisySetupFixture(), 30*time.Second)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	waitErr := proc.WaitAllReady(ctx, 30*time.Second)
	key := "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.NoisySetupSharedFixture"
	state := proc.State([]string{key})
	cancel()
	teardownErr := proc.Teardown()

	t.When("a shared fixture's setup prints to stdout, a fragment and a protocol-shaped line", func(w *gotest.T) {
		w.It("the fixture's state arrives whole and the run tears down clean", func(it *gotest.T) {
			gotest.NoError(it, waitErr)
			var got struct{ Marker string }
			gotest.NoError(it, json.Unmarshal(state[key], &got))
			gotest.Equal(it, "noisy", got.Marker)
			gotest.NoError(it, teardownErr)
		})
	})
}

func (s *SharedFixtureProtocolTestSuite) TestASecondDoneIsAProtocolError(t *gotest.T) {
	lines := `{"key":"_done","teardownBudget":"1m"}` + "\n" + `{"key":"_done","teardownBudget":"1m"}` + "\n"
	proc := gotestrunner.ExportReadProtocol(strings.NewReader(lines))
	t.When("the protocol stream carries _done twice", func(w *gotest.T) {
		w.It("the second is a protocol error, not a crash of the runner, and the setup stays good", func(it *gotest.T) {
			gotest.ErrorContains(it, gotestrunner.ExportProtocolErr(proc), "_done twice")
			gotest.NoError(it, proc.SetupErr())
		})
	})
}
