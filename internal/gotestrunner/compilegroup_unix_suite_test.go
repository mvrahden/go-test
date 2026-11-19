//go:build unix

package gotestrunner_test

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CompileGroupUnixTestSuite: where go renames its outputs into place, a
// binary runs as soon as it is there, while the go command still builds
// the rest. Windows judges a slot when go exits; the readiness rows cover it.
// Sequential: it replaces the go command the compile stage runs.
type CompileGroupUnixTestSuite struct{}

type pickupCtx struct {
	release chan struct{}
	restore []func()
}

func (s *CompileGroupUnixTestSuite) BeforeEach(t *gotest.T) *pickupCtx {
	ctx := &pickupCtx{release: make(chan struct{})}
	ctx.restore = []func(){
		gotestrunner.ExportSetGoTestC(func(_ context.Context, args, _ []string, _, _ io.Writer) error {
			stage, _ := fakeTarget(args)
			renameInto(stage, "m/fast", "built")
			<-ctx.release
			renameInto(stage, "m/slow", "built")
			return nil
		}),
		// A GOCACHE the work dir is not under: go renames.
		gotestrunner.ExportSetGoCacheDir("/nowhere/go-build"),
	}
	return ctx
}

func (s *CompileGroupUnixTestSuite) AfterEach(t *gotest.T, ctx *pickupCtx) {
	for i := len(ctx.restore) - 1; i >= 0; i-- {
		ctx.restore[i]()
	}
}

func (s *CompileGroupUnixTestSuite) TestBinariesArePickedUpWhileGoRuns(t *gotest.T, ctx *pickupCtx) {
	ch := gotestrunner.CompilePackagesStream(t.Context(), []string{"m/fast", "m/slow"}, "-overlay=o.json", nil, t.TempDir(), "", 0)
	// The method's Timeout fails it when the first outcome waits for the
	// go command.
	first := <-ch
	t.It("reports a package while the go command still runs", func(it *gotest.T) {
		gotest.Equal(it, "m/fast", first.Package)
		gotest.NoError(it, first.Err)
	})
	close(ctx.release)
	second := <-ch
	t.It("then the rest", func(it *gotest.T) { gotest.Equal(it, "m/slow", second.Package) })
	for range ch {
	}
}

// renameInto writes pkg's binary beside stage and renames it into it, as
// go does.
func renameInto(stage, pkg, content string) {
	tmp := filepath.Join(filepath.Dir(stage), "tmp-"+filepath.Base(binaryAt(stage, pkg)))
	_ = os.WriteFile(tmp, []byte(content), 0o600)
	_ = os.Rename(tmp, binaryAt(stage, pkg))
}
