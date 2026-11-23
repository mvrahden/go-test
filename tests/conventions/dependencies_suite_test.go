package conventions_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// DependencyAllowlistTestSuite lists what gotest brings into a user's
// build: the packages a test binary links, and the modules go.mod requires.
// A new entry is a decision, made by editing a list here.
type DependencyAllowlistTestSuite struct{}

func (s *DependencyAllowlistTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

// linked are the packages of this module a suite binary links through the
// two packages a harness or a suite imports.
var linked = []string{
	module + "/internal/callsite", module + "/internal/carry", module + "/internal/control", module + "/internal/fuzzcodec", module + "/internal/harness",
	module + "/internal/protocol", module + "/internal/protocol/sharedwire", module + "/internal/runstate",
	module + "/internal/schedinfo",
	module + "/internal/setupwaits", module + "/internal/unidiff",
	module + "/pkg/gotest", module + "/pkg/gotest/internal/assert", module + "/pkg/gotest/internal/snapfile",
	module + "/pkg/gotestruntime",
}

// TestWhatATestBinaryLinks compares every system before failing, so one
// failure names each system a package joined or left.
func (s *DependencyAllowlistTestSuite) TestWhatATestBinaryLinks(t *gotest.T) {
	var changed []string
	for _, goos := range []string{"linux", "darwin", "windows", "freebsd"} {
		deps := listDeps(t, goos, "amd64", "./pkg/gotest", "./pkg/gotestruntime")
		for _, dep := range deps {
			if !slices.Contains(linked, dep) {
				changed = append(changed, goos+": joined "+dep)
			}
		}
		for _, dep := range linked {
			if !slices.Contains(deps, dep) {
				changed = append(changed, goos+": left "+dep)
			}
		}
	}
	gotest.Empty(t, changed, "what every user's test binary links changed: edit linked in tests/conventions/dependencies_suite_test.go on purpose")
}

func (s *DependencyAllowlistTestSuite) TestWhatTheModuleRequires(t *gotest.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	gotest.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "go", "mod", "edit", "-json")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	gotest.NoError(t, err, "go mod edit: %s", stderr.String())
	var mod struct {
		Require []struct {
			Path     string
			Indirect bool
		}
	}
	gotest.NoError(t, json.Unmarshal(out, &mod))
	var direct, indirect []string
	for _, r := range mod.Require {
		if r.Indirect {
			indirect = append(indirect, r.Path)
		} else {
			direct = append(direct, r.Path)
		}
	}
	gotest.ElementsMatch(t, []string{
		"github.com/fsnotify/fsnotify", "go.yaml.in/yaml/v4", "golang.org/x/mod", "golang.org/x/sys", "golang.org/x/tools",
	}, direct, "go.mod requires another set of modules: each one reaches every user's go.mod through the tool directive")
	gotest.ElementsMatch(t, []string{"golang.org/x/sync"}, indirect, "go.mod's indirect requirements changed: edit the list in this file on purpose")
}
