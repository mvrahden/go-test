// Package latepanic_test abandons a hanging poll, lets it panic, and echoes
// what gotest wrote to stderr for it.
package latepanic_test

import (
	"os"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type LatePanicTestSuite struct{}

// TestAbandonedPollPanics reads stderr through a pipe until the panic shows,
// or for 10s, then writes it to the real stderr. os.Stderr stays the pipe:
// the abandoned poll reads it with no ordering against a restore.
func (s *LatePanicTestSuite) TestAbandonedPollPanics(t *gotest.T) {
	r, w, err := os.Pipe()
	gotest.NoError(t, err)
	stderr := os.Stderr
	os.Stderr = w
	release := make(chan struct{})
	m := gotest.Record(func(rec *gotest.R) {
		gotest.Eventually(rec, 20*time.Millisecond, 10*time.Millisecond, func(*gotest.R) {
			<-release
			panic("late boom")
		})
	})
	gotest.True(t, m.Failed(), "the hanging poll was not abandoned")
	close(release)
	got := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(b.String(), "latepanic_suite_test.go") { // the panicking frame
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		got <- b.String()
	}()
	select {
	case out := <-got:
		_, _ = stderr.WriteString(out)
	case <-time.After(10 * time.Second):
		gotest.Fail(t, "nothing was written to stderr for the abandoned poll's panic")
	}
}
