// Package goversion refuses to run a gotest binary built by an older Go than
// the module declares, naming the binary and the fix instead of the type
// checker's "application" message. Only a stale `go install` binary trips it.
package goversion

import (
	"bytes"
	"fmt"
	"go/version"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/tools/go/packages"
)

// Check compares this binary's Go with each package's module go directive.
// Packages must be loaded with packages.NeedModule.
func Check(pkgs []*packages.Package) error {
	return check(pkgs, runtime.Version(), selectedVersion)
}

func check(pkgs []*packages.Package, builtWith string, sel selector) error {
	built := version.Lang(builtWith)
	if built == "" {
		return nil
	}
	for _, p := range pkgs {
		if p.Module == nil || p.Module.GoVersion == "" {
			continue
		}
		declared := version.Lang("go" + p.Module.GoVersion)
		if declared == "" || version.Compare(built, declared) >= 0 {
			continue
		}
		exe, _ := os.Executable()
		return fmt.Errorf("%s %s (%s) was built with %s, but module %s declares go %s; %s",
			about.Application, about.ResolvedVersion(), exe, builtWith, p.Module.Path, p.Module.GoVersion, fix(pkgs, p.Module, sel))
	}
	return nil
}

// selector reports the go-test version the module in dir selects.
type selector func(dir string) (string, selection)

// selection is what the module says about go-test.
type selection int

const (
	selected    selection = iota // it selects a version, "" for a workspace module
	notRequired                  // it does not depend on go-test
	unknown                      // the go command could not tell
)

// fix tells how to run the CLI through the tool directive at the go-test
// version the module selects: never this binary's, which may be older, and
// never @latest, which can raise the go line.
func fix(pkgs []*packages.Package, mod *packages.Module, sel selector) string {
	const run = "run the CLI through the tool directive so the module's Go builds it: "
	const declare = "declare the tool at a release whose go line the module takes (go get -tool " + about.Repo + "/cmd/gotest@VERSION, see README, Install), then run go tool gotest ./..."
	v, s := pinnedVersion(pkgs, mod, sel)
	switch {
	case s == notRequired:
		return fmt.Sprintf("module %s does not require go-test: %s", mod.Path, declare)
	case s == unknown:
		return declare
	case v == "":
		// A workspace module: the workspace selects it, there is no pin.
		return run + "go get -tool " + about.Repo + "/cmd/gotest && go tool gotest ./... (see README, Install)"
	default:
		return run + "go get -tool " + about.Repo + "/cmd/gotest@" + v + " && go tool gotest ./... (see README, Install)"
	}
}

// pinnedVersion is the go-test version the build selects: the one the
// loaded packages import, else the one the module requires, as go.work and
// replace directives resolve it.
func pinnedVersion(pkgs []*packages.Package, mod *packages.Module, sel selector) (string, selection) {
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if m := imp.Module; m != nil && m.Path == about.Repo && m.Version != "" {
				return m.Version, selected
			}
		}
	}
	return sel(mod.Dir)
}

// selectedVersion asks the go command which go-test version the module in
// dir selects.
func selectedVersion(dir string) (string, selection) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", about.Repo) //nolint:gosec // G204: the go tool with fixed arguments
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "not a known dependency") {
			return "", notRequired
		}
		return "", unknown
	}
	return strings.TrimSpace(stdout.String()), selected
}
