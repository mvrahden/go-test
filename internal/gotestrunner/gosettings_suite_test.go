package gotestrunner_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoSettingsTestSuite: the go command's own GOOS, GOARCH and GOFLAGS, and
// what a failed start says about them. Sequential: it sets the environment.
type GoSettingsTestSuite struct{}

func (s *GoSettingsTestSuite) TestRead(t *gotest.T) {
	t.It("takes them from the environment", func(it *gotest.T) {
		it.Setenv("GOOS", "windows")
		it.Setenv("GOARCH", "arm64")
		it.Setenv("GOFLAGS", "-tags=x")
		gotest.Equal(it, gotestrunner.GoSettings{GOOS: "windows", GOARCH: "arm64", GOFLAGS: "-tags=x"}, gotestrunner.ExportReadGoSettings())
	})
	t.It("takes them from go's env file where the environment is silent", func(it *gotest.T) {
		file := filepath.Join(it.TempDir(), "go.env")
		gotest.NoError(it, os.WriteFile(file, []byte("GOOS=windows\nGOARCH=arm64\nGOFLAGS=-n\n"), 0o600))
		it.Setenv("GOENV", file)
		it.Setenv("GOOS", "")
		it.Setenv("GOARCH", "")
		it.Setenv("GOFLAGS", "")
		gotest.Equal(it, gotestrunner.GoSettings{GOOS: "windows", GOARCH: "arm64", GOFLAGS: "-n"}, gotestrunner.ExportReadGoSettings())
	})
	t.It("falls back to the host and the environment without a go command", func(it *gotest.T) {
		it.Setenv("PATH", it.TempDir())
		it.Setenv("GOFLAGS", "-x")
		gotest.Equal(it, gotestrunner.GoSettings{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GOFLAGS: "-x"}, gotestrunner.ExportReadGoSettings())
	})
}

func (s *GoSettingsTestSuite) TestForeignTargetNote(t *gotest.T) {
	const note = "gotest: the test binary is built for windows/arm64 and this host is linux/amd64: unset GOOS and GOARCH to test on the host, or give -exec a command that runs the binary\n"
	for sub, r := range gotest.Each(t, []struct {
		Desc         string
		GOOS, GOARCH string
		Exec         []string
		Want         string
	}{
		{Desc: "the host's platform", GOOS: "linux", GOARCH: "amd64", Want: ""},
		{Desc: "another system and architecture", GOOS: "windows", GOARCH: "arm64", Want: note},
		{Desc: "another architecture alone", GOOS: "linux", GOARCH: "arm64", Want: "gotest: the test binary is built for linux/arm64 and this host is linux/amd64: unset GOOS and GOARCH to test on the host, or give -exec a command that runs the binary\n"},
		{Desc: "another platform under -exec", GOOS: "windows", GOARCH: "arm64", Exec: []string{"wine"}, Want: ""},
	}) {
		got := gotestrunner.ExportForeignTargetNote(gotestrunner.GoSettings{GOOS: r.GOOS, GOARCH: r.GOARCH}, "linux", "amd64", r.Exec)
		gotest.Equal(sub, r.Want, got)
	}
}
