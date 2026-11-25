package gotestrunner_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditBaseTestSuite: one audit of a checkout at a time holds its export
// directory, and clearing the cache leaves a held one in place.
// Sequential: Setenv.
type AuditBaseTestSuite struct{}

func (s *AuditBaseTestSuite) TestOneAuditAtATime(t *gotest.T) {
	t.When("an audit holds a checkout's directory", func(w *gotest.T) {
		w.It("keeps a second audit of that checkout waiting until it lets go, and none of another", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, it.TempDir())
			first := gotest.Must(gotestrunner.LockAuditBase("/checkout/a"))
			other := gotest.Must(gotestrunner.LockAuditBase("/checkout/b"))
			gotest.NotEqual(it, first.Dir, other.Dir)
			other.Release()

			var got atomic.Pointer[gotestrunner.AuditBase]
			gotest.Go(it, func() {
				got.Store(gotest.Must(gotestrunner.LockAuditBase("/checkout/a")))
			})
			gotest.Consistently(it, 300*time.Millisecond, 20*time.Millisecond, func(poll *gotest.R) {
				gotest.Zero(poll, got.Load())
			})
			first.Release()
			gotest.Eventually(it, 5*time.Second, 20*time.Millisecond, func(poll *gotest.R) {
				gotest.NotZero(poll, got.Load())
			})
			gotest.Equal(it, first.Dir, got.Load().Dir)
			got.Load().Release()
		})
	})
	t.When("the cache is cleared while an audit holds a directory", func(w *gotest.T) {
		w.It("removes the others and leaves that one", func(it *gotest.T) {
			it.Setenv(protocol.EnvCacheDir, it.TempDir())
			held := gotest.Must(gotestrunner.LockAuditBase("/checkout/a"))
			idle := gotest.Must(gotestrunner.LockAuditBase("/checkout/b"))
			for _, b := range []*gotestrunner.AuditBase{held, idle} {
				gotest.NoError(it, os.MkdirAll(filepath.Join(b.Dir, "tree"), 0o750))
			}
			idle.Release()
			removed, inUse, _, err := gotestrunner.ClearAuditBases()
			gotest.NoError(it, err)
			gotest.Equal(it, []int{1, 1}, []int{removed, inUse})
			_, err = os.Stat(held.Dir)
			gotest.NoError(it, err)
			_, err = os.Stat(idle.Dir)
			gotest.ErrorIs(it, err, os.ErrNotExist)
			held.Release()
		})
	})
}
