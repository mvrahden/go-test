package wrapped

import (
	"bytes"
	"os"
	"runtime"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// verify stands in for goleak.VerifyTestMain: it runs the tests itself through
// the interface it takes, then checks something about the whole process.
func verify(m interface{ Run() int }) {
	code := m.Run()
	// As goleak does: nothing gotest started may be left running.
	buf := make([]byte, 1<<20)
	if bytes.Contains(buf[:runtime.Stack(buf, true)], []byte("gotestruntime.")) {
		logLine("goroutine left")
	}
	logLine("verified")
	os.Exit(code)
}

func TestMain(m *testing.M) { verify(gotestruntime.M(m)) }
