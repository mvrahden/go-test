package conventions_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ImportsTestSuite holds what a user's test binary links to the standard
// library and this module, on every platform Go lists, and keeps the CLI
// off the public packages. A third-party import there is one go mod tidy
// cannot see: only the generated harness imports the runtime.
type ImportsTestSuite struct{}

func (s *ImportsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const module = "github.com/mvrahden/go-test"

// runtimePackages are the packages a suite and its harness import.
var runtimePackages = []string{"./pkg/gotest/...", "./pkg/gotestruntime/...", "./pkg/gotestfuzz/..."}

// listDeps lists the non-standard packages patterns depend on, for one platform.
func listDeps(t *gotest.T, goos, goarch string, patterns ...string) []string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	args := append([]string{"list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}"}, patterns...)
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	gotest.NoError(t, err, "go list: %s", stderr.String())
	var deps []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			deps = append(deps, line)
		}
	}
	return deps
}

func (s *ImportsTestSuite) TestTheRuntimeLinksOnlyTheStandardLibraryAndThisModule(t *gotest.T) {
	for sub, row := range gotest.Each(t, []struct{ Desc, goos, goarch string }{
		{Desc: "linux/amd64", goos: "linux", goarch: "amd64"},
		{Desc: "linux/arm64", goos: "linux", goarch: "arm64"},
		{Desc: "darwin/arm64", goos: "darwin", goarch: "arm64"},
		{Desc: "windows/amd64", goos: "windows", goarch: "amd64"},
		{Desc: "freebsd/amd64", goos: "freebsd", goarch: "amd64"},
		{Desc: "netbsd/amd64", goos: "netbsd", goarch: "amd64"},
		{Desc: "openbsd/amd64", goos: "openbsd", goarch: "amd64"},
		{Desc: "dragonfly/amd64", goos: "dragonfly", goarch: "amd64"},
		{Desc: "solaris/amd64", goos: "solaris", goarch: "amd64"},
		{Desc: "illumos/amd64", goos: "illumos", goarch: "amd64"},
		{Desc: "aix/ppc64", goos: "aix", goarch: "ppc64"},
	}) {
		var foreign []string
		for _, dep := range listDeps(sub, row.goos, row.goarch, runtimePackages...) {
			if dep != module && !strings.HasPrefix(dep, module+"/") {
				foreign = append(foreign, dep)
			}
		}
		gotest.Empty(sub, foreign, "a user's test binary links these; go mod tidy drops their go.sum lines from a module without the tool directive, so use the standard library")
	}
}

func (s *ImportsTestSuite) TestTheCLIImportsNoPublicPackage(t *gotest.T) {
	var public []string
	for _, dep := range listDeps(t, runtime.GOOS, runtime.GOARCH, "./cmd/gotest") {
		if strings.HasPrefix(dep, module+"/pkg/") {
			public = append(public, dep)
		}
	}
	gotest.Empty(t, public, "cmd/gotest must not import pkg/*: the CLI and the runtime a module requires are versioned apart")
}
