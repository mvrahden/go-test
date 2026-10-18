package gotestgen_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

const budgetedTeardownKey = "github.com/mvrahden/go-test/tests/sharedfixture/fixtures.BudgetedTeardownSharedFixture"

// lateBudgetReports runs the generated setup program for a fixture held for a
// window, declaring timeout: it starts the fixture with a start command, then
// closes stdin. It returns the teardown budgets the _done line and the start's
// acknowledgement reported.
func lateBudgetReports(t *gotest.T, timeout time.Duration) (done, ack string) {
	src, err := gotestgen.GenerateSharedSetup([]gotestgen.SharedFixtureInfo{{
		Identifier:     "BudgetedTeardownSharedFixture",
		HasAfterAll:    true,
		HasConfig:      true,
		PkgPath:        "github.com/mvrahden/go-test/tests/sharedfixture/fixtures",
		PkgName:        "fixtures",
		QualifiedType:  "fixtures.BudgetedTeardownSharedFixture",
		TransferFields: []string{"Marker"},
		Deferred:       true,
	}})
	gotest.NoError(t, err)
	dir := t.TempDir()
	file, bin := filepath.Join(dir, "setup.go"), filepath.Join(dir, "setup")
	gotest.NoError(t, os.WriteFile(file, src, 0o600))
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", bin, file).CombinedOutput() //nolint:gosec // G204: the program this test generated
	gotest.NoError(t, err, "build: %s", out)

	cmd := exec.CommandContext(t.Context(), bin) //nolint:gosec // G204: the program this test built
	cmd.Env = append(os.Environ(), fixtures.EnvBudgetedTeardownTimeout+"="+timeout.String())
	stdin, err := cmd.StdinPipe()
	gotest.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	cmd.Stderr = io.Discard
	gotest.NoError(t, cmd.Start())

	lines := make(chan map[string]string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var line map[string]string
			if json.Unmarshal(sc.Bytes(), &line) == nil {
				lines <- line
			}
		}
	}()
	// The budget of the first line keyed key, or "" when none came in time.
	await := func(key string) string {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					return ""
				}
				if line["key"] == key {
					return line["teardownBudget"]
				}
			case <-timer.C:
				return ""
			}
		}
	}
	done = await("_done")
	_, err = io.WriteString(stdin, `{"verb":"start","keys":["`+budgetedTeardownKey+`"]}`+"\n")
	if err == nil {
		ack = await("_cmd")
	}
	_ = stdin.Close()
	for range lines {
	}
	_ = cmd.Wait()
	return done, ack
}

func (s *SharedFixtureTestSuite) TestALateFixturesTeardownBudget(t *gotest.T) {
	done, ack := lateBudgetReports(t, 3*time.Minute)
	t.When("a shared fixture declaring a 3m Timeout is held for a window and then started", func(w *gotest.T) {
		w.It("the up-front report counts no fixture that has not started", func(it *gotest.T) {
			gotest.Equal(it, "30s", done)
		})
		w.It("the start's acknowledgement reports a budget its 3m AfterAll fits in", func(it *gotest.T) {
			gotest.Equal(it, "3m30s", ack)
		})
	})
}
