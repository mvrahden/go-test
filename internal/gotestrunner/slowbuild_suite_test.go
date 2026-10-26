package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowBuildThresholdTestSuite: GOTEST_SLOW_BUILD_THRESHOLD moves the
// slow-build notice.
//
// Sequential: Setenv, and it swaps os.Stderr.
type SlowBuildThresholdTestSuite struct{}

func (s *SlowBuildThresholdTestSuite) TestTheVariableSetsTheThreshold(t *gotest.T) {
	entries := []struct {
		Desc  string
		value string
		unset bool
		want  time.Duration
	}{
		{Desc: "unset", unset: true, want: 15 * time.Second},
		{Desc: "empty", value: "", want: 15 * time.Second},
		{Desc: "a nanosecond", value: "1ns", want: time.Nanosecond},
		{Desc: "minutes", value: "2m", want: 2 * time.Minute},
		{Desc: "not a duration", value: "15", want: 15 * time.Second},
		{Desc: "zero", value: "0s", want: 15 * time.Second},
		{Desc: "negative", value: "-1s", want: 15 * time.Second},
	}
	for t, entry := range gotest.Each(t, entries) {
		// Setenv first, so the variable is restored after the row.
		t.Setenv(protocol.EnvSlowBuildThreshold, entry.value)
		if entry.unset {
			gotest.NoError(t, os.Unsetenv(protocol.EnvSlowBuildThreshold))
		}
		gotest.Equal(t, entry.want, gotestrunner.ExportSlowBuildThreshold())
	}
}

// stderrOf is what f writes to os.Stderr.
func stderrOf(t *gotest.T, f func()) string {
	path := filepath.Join(t.TempDir(), "stderr")
	file, err := os.Create(path)
	gotest.NoError(t, err)
	saved := os.Stderr
	os.Stderr = file
	f()
	os.Stderr = saved
	gotest.NoError(t, file.Close())
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	return string(data)
}

// missing is a package of this module that does not exist: its build starts,
// fails on its own and lasts as long as the go command takes to say so. No
// deadline cuts it short, since one that ends before it starts leaves no
// build to notice.
const missing = "github.com/mvrahden/go-test/internal/gotestrunner/testdata/missing"

func (s *SlowBuildThresholdTestSuite) TestATestBinaryBuildHonorsTheVariable(t *gotest.T) {
	t.Setenv(protocol.EnvSlowBuildThreshold, "1ns")
	out := stderrOf(t, func() {
		_, err := gotestrunner.ExportCompilePackage(context.Background(), missing, "-overlay=", nil, t.TempDir(), "")
		gotest.Error(t, err)
	})
	gotest.Contains(t, out, "gotest: test binary for "+missing+" finished building after")
}

func (s *SlowBuildThresholdTestSuite) TestASharedFixtureBuildHonorsTheVariable(t *gotest.T) {
	t.Setenv(protocol.EnvSlowBuildThreshold, "1ns")
	fixtures := []gotestgen.SharedFixtureInfo{{
		Identifier:    "MissingSharedFixture",
		PkgPath:       missing,
		PkgName:       "missing",
		QualifiedType: "missing.MissingSharedFixture",
	}}
	out := stderrOf(t, func() {
		_, err := gotestrunner.StartSharedFixtures(context.Background(), t.TempDir(), fixtures, time.Minute)
		gotest.Error(t, err)
	})
	gotest.Contains(t, out, "gotest: shared fixture setup binary finished building after")
}
