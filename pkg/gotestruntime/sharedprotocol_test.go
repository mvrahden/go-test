package gotestruntime_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// SharedProtocolTestSuite runs a shared fixture process's claim of its
// protocol descriptor in a child of the test binary.
type SharedProtocolTestSuite struct{}

func (s *SharedProtocolTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the protocol descriptor is passed on Unix only"
	}
	return ""
}

func (s *SharedProtocolTestSuite) TestDescriptor(t *gotest.T) {
	t.When("a fixture starts a process of its own", func(w *gotest.T) {
		w.It("hands it neither the protocol descriptor nor the variable naming it", func(it *gotest.T) {
			if out := os.Getenv("GOTEST_PROTOCOL_CHILD_OUT"); out != "" {
				proto := gotestruntime.ClaimSharedProtocol()
				f, err := os.Create(out) //nolint:gosec // G304: test-only path
				gotest.NoError(it, err)
				grandchild := exec.Command("sh", "-c", `printf 'env=%s\n' "$`+protocol.EnvSharedProtocolFD+`"; echo injected >&3`) //nolint:gosec // G204: fixed args
				grandchild.Stdout = f
				_ = grandchild.Run()
				gotest.NoError(it, f.Close())
				_, err = proto.WriteString("own\n")
				gotest.NoError(it, err)
				return
			}

			out := filepath.Join(it.TempDir(), "grandchild")
			r, pw, err := os.Pipe()
			gotest.NoError(it, err)
			child := exec.Command(os.Args[0], //nolint:gosec // G204: test-only subprocess
				"-test.run=^TestSharedProtocolTestSuite$/^TestDescriptor$")
			child.Env = append(os.Environ(),
				"GOTEST_PROTOCOL_CHILD_OUT="+out,
				protocol.EnvSharedProtocolFD+"=3")
			child.ExtraFiles = []*os.File{pw}
			gotest.NoError(it, child.Start())
			gotest.NoError(it, pw.Close())
			stream, err := io.ReadAll(r)
			gotest.NoError(it, err)
			gotest.NoError(it, r.Close())
			gotest.NoError(it, child.Wait())

			gotest.Equal(it, "own\n", string(stream))
			seen, err := os.ReadFile(out) //nolint:gosec // G304: test-only path
			gotest.NoError(it, err)
			gotest.Equal(it, "env=\n", string(seen))
		})
	})
}
