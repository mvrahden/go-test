//go:build unix

package channel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ChannelTestSuite starts processes of its own from its test binary.
type ChannelTestSuite struct{}

// TestAProcessItStarts counts the sockets a process it starts inherits
// against its own: the host may hand every process a socket or two, so the
// channel shows as the one socket the child lacks.
func (s *ChannelTestSuite) TestAProcessItStarts(t *gotest.T) {
	mark(fmt.Sprintf("suite sockets=%d", countSockets()))
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "CHANNEL_PROBE=inherited")
	gotest.NoError(t, cmd.Run())
}

func (s *ChannelTestSuite) TestADescriptorThatIsNoChannel(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "fd3")
	f, err := os.Create(path)
	gotest.NoError(t, err)
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "CHANNEL_PROBE=descriptor", "GOTEST_CONTROL=3")
	cmd.ExtraFiles = []*os.File{f}
	err = cmd.Run()
	_ = f.Close()
	gotest.NoError(t, err)
	got, err := os.ReadFile(path)
	gotest.NoError(t, err)
	if string(got) == "kept\n" {
		mark("descriptor kept")
	}
}

// TestMain probes in a process a test started; otherwise it runs the tests
// and marks whether a reader of the channel outlived them.
func TestMain(m *testing.M) {
	if kind := os.Getenv("CHANNEL_PROBE"); kind != "" {
		probe(kind)
		os.Exit(0)
	}
	code := m.Run()
	if readerLeft() {
		mark("reader left")
	} else {
		mark("no reader left")
	}
	os.Exit(code)
}
