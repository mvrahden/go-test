// Package speccatalog reads the behavioral promises in specs/ and holds the
// conformance suites to them. gotest scenarios binds each scenario to the
// suites' When/It labels and gotest spec --static lists their tree, both
// read from source when asked; the rules this repository adds on top of
// that binding live here.
package speccatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestscenarios"
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
)

// Pattern selects every conformance package.
const Pattern = "./tests/conformance/..."

// ScenariosArgs is the gotest invocation whose document binds specs/ to the
// conformance suites, the Its no clause states listed.
var ScenariosArgs = []string{"scenarios", "--specs=" + SpecsDir, "--unstated", "--format=json", Pattern}

// StaticArgs is the gotest invocation that lists the conformance suites'
// tree from source.
var StaticArgs = []string{"spec", "--static", "--format=json", Pattern}

// Reading is what gotest reads of the conformance suites: the scenarios
// document and the tree.
type Reading struct {
	Scenarios gotestscenarios.Document
	Packages  []Package
}

// Package is one conformance package of the tree, as spec --static lists it.
type Package struct {
	Path  string `json:"path"`
	Nodes []node `json:"nodes"`
}

// capability is the package's directory under tests/conformance.
func (p *Package) capability() string {
	_, c, _ := strings.Cut(p.Path, "/"+ConformanceDir+"/")
	return c
}

// Read runs both invocations on root with the gotest built from root's
// sources.
func Read(ctx context.Context, root string) (Reading, error) {
	return read(func(args []string) (string, string, int, error) {
		r, err := testkit.RunCLIWithEnv(ctx, root, []string{linux}, args...)
		return r.Stdout, r.Stderr, r.Exit, err
	})
}

// ReadWith runs both invocations on root with the gotest binary given.
func ReadWith(ctx context.Context, binary, root string) (Reading, error) {
	return read(func(args []string) (string, string, int, error) {
		cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // G204: the gotest the caller built
		cmd.Dir = root
		cmd.Env = append(os.Environ(), linux)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.String(), stderr.String(), exit.ExitCode(), nil
		}
		return stdout.String(), stderr.String(), 0, err
	})
}

// linux is how both invocations see the source on every host: a
// build-constrained conformance file (unix-only, say) would otherwise bind
// on one host and not on another.
const linux = "GOOS=linux"

// read runs both invocations at once and decodes what they print.
func read(run func(args []string) (stdout, stderr string, exit int, err error)) (Reading, error) {
	type result struct {
		out string
		err error
	}
	start := func(args []string) <-chan result {
		ch := make(chan result, 1)
		go func() {
			stdout, stderr, exit, err := run(args)
			switch {
			case err != nil:
			case exit != 0:
				err = exitError(args[0], exit, stderr)
			case stdout == "":
				err = errors.New("gotest " + args[0] + " printed nothing")
			}
			ch <- result{stdout, err}
		}()
		return ch
	}
	scenarios, static := start(ScenariosArgs), start(StaticArgs)
	s, t := <-scenarios, <-static
	if err := errors.Join(s.err, t.err); err != nil {
		return Reading{}, err
	}
	return Decode([]byte(s.out), []byte(t.out))
}

// exitError says a gotest command exited with exit, a Windows termination
// status in hex with its name, and what it wrote to stderr.
func exitError(command string, exit int, stderr string) error {
	return fmt.Errorf("gotest %s exited %s:\n%s", command, proctree.ExitStatus(exit), stderr)
}

// Decode reads a scenarios document and a spec --static document.
func Decode(scenarios, static []byte) (Reading, error) {
	var r Reading
	if err := json.Unmarshal(scenarios, &r.Scenarios); err != nil {
		return Reading{}, fmt.Errorf("scenarios json: %w", err)
	}
	var tree struct {
		Packages []Package `json:"packages"`
	}
	if err := json.Unmarshal(static, &tree); err != nil {
		return Reading{}, fmt.Errorf("spec --static json: %w", err)
	}
	r.Packages = tree.Packages
	return r, nil
}
