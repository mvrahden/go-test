// Package other has an Exclusive suite of the module the hold package
// belongs to, in a directory of its own.
package other

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(os.Getenv("CROSSRUN_ROLE") + " " + event + "\n")
}

type OtherTestSuite struct{}

func (s *OtherTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true}
}

func (s *OtherTestSuite) TestRuns(t *gotest.T) {
	t.AssertsBy("it stamps markers the run under test reads back")
	mark("other begins")
	mark("other ends")
}
