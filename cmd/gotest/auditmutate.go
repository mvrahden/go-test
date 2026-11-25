package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/mutate"
	"github.com/mvrahden/go-test/internal/protocol"
)

// defaultMutateBudget is the time audit --mutate gives the mutants when
// --mutate-budget is not given: about a hundred changed lines of suites
// that start processes.
const defaultMutateBudget = 3 * time.Minute

// maxBuildRounds bounds the builds that drop the mutants a compiler
// refuses.
const maxBuildRounds = 5

// Notes of the mutation section, each said once.
const (
	schemataNote = "--mutate: every mutant is compiled into one build and switched on by GOTEST_MUTANT; the suites that execute a mutant ran on that build with none switched on before any mutant did. A suite that fails there but passes in the run above is timing-sensitive or reads its own code"
	binaryNote   = "--mutate: a line only a prebuilt binary or a container executes reads as not covered; a binary a test builds from the module counts when built with go build, adding -cover only where GOCOVERDIR is set: the mutants reach it through GOFLAGS, and go build -cover compiles from disk without them"
)

// mutation is one audit --mutate: the directory it holds, the mutants and
// what kept or dropped each.
type mutation struct {
	held *gotestrunner.AuditBase
	dir  string
	// overlay is the mutants' overlay file, fixed per checkout.
	overlay string
	plan    *mutate.Plan
	// all is every mutant of the changed lines, by id.
	all  []mutate.Mutant
	byID map[int]*mutate.Mutant
	// ignored gives an ignored mutant's reason; refused the mutants the
	// compiler refused.
	ignored map[int]string
	refused map[int]bool
	files   map[string]*gotestaudit.ProdFile // by absolute path
	lines   map[string][]string              // each file's source lines
	ignores []gotestaudit.MutateIgnore
	// pkgs are the mutated packages; tests the packages holding tests.
	pkgs, tests []string
	notes       []string
	// exit is the coverage run's exit code.
	exit int
}

func (m *mutation) release() {
	if m != nil && m.held != nil {
		m.held.Release()
	}
}

// suiteDir is where the baseline keeps each suite's coverage.
func (m *mutation) suiteDir() string { return filepath.Join(m.dir, "suites") }

// prepareMutation writes the mutants of the changed files into an overlay
// of their own and checks that it compiles, dropping the mutants a compiler
// refuses. nil, with a note in the document, when nothing is to mutate.
func (a *auditArgs) prepareMutation(doc *gotestaudit.Document, files []gotestaudit.ProdFile) (*mutation, error) {
	if len(files) == 0 {
		doc.Notes = append(doc.Notes, "--mutate: no changed production line, so nothing was mutated")
		return nil, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	held, err := gotestrunner.LockAuditMutate(cwd)
	if err != nil {
		return nil, fmt.Errorf("--mutate: the mutants' directory: %w", err)
	}
	m := &mutation{held: held, dir: held.Dir, files: map[string]*gotestaudit.ProdFile{}, lines: map[string][]string{}, ignored: map[int]string{}, refused: map[int]bool{}}
	if err := os.RemoveAll(m.dir); err != nil {
		m.release()
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(m.dir, "src"), 0o700); err != nil {
		m.release()
		return nil, err
	}
	m.overlay = filepath.Join(m.dir, "overlay.json")
	if err := m.generate(a, cwd, files); err != nil {
		m.release()
		return nil, err
	}
	if len(m.all) == 0 {
		doc.Notes = append(doc.Notes, "--mutate: no operator applies to the changed lines, so nothing was mutated")
		m.release()
		return nil, nil
	}
	return m, nil
}

// generate writes the plan of the changed files, its ignored and refused
// mutants dropped.
func (m *mutation) generate(a *auditArgs, cwd string, files []gotestaudit.ProdFile) error {
	changed := mutate.Changed{}
	for i := range files {
		f := &files[i]
		m.files[f.Abs] = f
		lines := map[int]bool{}
		for _, l := range f.Lines {
			lines[l] = true
		}
		changed[f.Abs] = lines
		src, err := os.ReadFile(f.Abs)
		if err != nil {
			return err
		}
		m.lines[f.Abs] = strings.Split(string(src), "\n")
		for _, ig := range mutate.Ignores(f.Abs, src) {
			m.ignores = append(m.ignores, gotestaudit.MutateIgnore{Module: f.Module, File: f.File, Line: ig.Line, Scope: ig.Scope, Start: ig.Start, End: ig.End, Func: ig.Func, Reason: ig.Reason})
			if ig.Reason == "" {
				m.notes = append(m.notes, fmt.Sprintf("--mutate: %s:%d: %s without a reason keeps no line", f.File, ig.Line, mutate.IgnoreDirective))
			}
		}
	}
	pkgs, err := mutate.Load(cwd, changed, a.loadFlags)
	if err != nil {
		return fmt.Errorf("--mutate: loading the changed packages: %w", err)
	}
	opt := mutate.Options{Strings: a.mutateStrings, Drop: map[int]bool{}}
	full, err := mutate.Generate(pkgs, changed, opt)
	if err != nil {
		return fmt.Errorf("--mutate: %w", err)
	}
	m.all = full.Mutants
	m.byID = map[int]*mutate.Mutant{}
	for i := range m.all {
		mu := &m.all[i]
		m.byID[mu.ID] = mu
		if r := m.ignoreOf(mu); r != "" {
			m.ignored[mu.ID] = r
			opt.Drop[mu.ID] = true
		}
		if !slices.Contains(m.pkgs, mu.Package) {
			m.pkgs = append(m.pkgs, mu.Package)
		}
	}
	slices.Sort(m.pkgs)
	for _, s := range full.Skipped {
		m.notes = append(m.notes, "--mutate: no mutant in "+relPath(cwd, s))
	}
	return m.build(a, pkgs, changed, opt)
}

// ignoreOf is the reason of the comment keeping mu's line, "" for none.
func (m *mutation) ignoreOf(mu *mutate.Mutant) string {
	f := m.files[mu.File]
	for i := range m.ignores {
		ig := &m.ignores[i]
		if f != nil && ig.File == f.File && ig.Module == f.Module && ig.Reason != "" && ig.Start <= mu.Line && mu.Line <= ig.End {
			return ig.Reason
		}
	}
	return ""
}

// build renders the plan and compiles the mutated packages with it,
// dropping the mutants of the lines the compiler refuses, for a few rounds.
func (m *mutation) build(a *auditArgs, pkgs []*packages.Package, changed mutate.Changed, opt mutate.Options) error {
	for round := 1; ; round++ {
		plan, err := mutate.Generate(pkgs, changed, opt)
		if err != nil {
			return fmt.Errorf("--mutate: %w", err)
		}
		m.plan = plan
		if len(plan.Mutants) == 0 {
			return nil
		}
		if err := m.writeOverlay(); err != nil {
			return err
		}
		args := append([]string{"build", "-o", os.DevNull, "-overlay=" + m.overlay}, a.loadFlags...)
		cmd := exec.Command("go", append(args, m.pkgs...)...) //nolint:gosec // G204: go build of the mutated packages
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		cwd, _ := os.Getwd()
		bad := mutate.Refused(out, cwd, plan.Mutants)
		if len(bad) == 0 || round == maxBuildRounds {
			return fmt.Errorf("--mutate: the build holding the mutants does not compile:\n%s", strings.TrimSpace(string(out)))
		}
		for _, id := range bad {
			opt.Drop[id] = true
			m.refused[id] = true
		}
	}
}

// writeOverlay writes the plan's files and the overlay naming them.
func (m *mutation) writeOverlay() error {
	src := filepath.Join(m.dir, "src")
	replace := map[string]string{}
	names := make([]string, 0, len(m.plan.Files))
	for name := range m.plan.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	for i, name := range names {
		p := filepath.Join(src, fmt.Sprintf("%d_%s", i, filepath.Base(name)))
		if err := os.WriteFile(p, m.plan.Files[name], 0o600); err != nil {
			return err
		}
		replace[name] = p
	}
	data, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		return err
	}
	return os.WriteFile(m.overlay, data, 0o600)
}

// testPackages lists the packages the patterns match that hold tests: a
// test may reach a mutated line without importing its package, through a
// binary it builds and starts, so coverage, not imports, picks the suites.
func (m *mutation) testPackages(a *auditArgs) error {
	args := append([]string{"list", "-e", "-test", "-f={{.ForTest}}"}, a.loadFlags...)
	cmd := exec.Command("go", append(args, a.patterns...)...) //nolint:gosec // G204: go list of the audit's own patterns
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("--mutate: go list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	for _, p := range strings.Fields(string(out)) {
		m.tests = appendOnce(m.tests, p)
	}
	slices.Sort(m.tests)
	return nil
}

// runEnv is the environment of a run on the mutants' build with mutant id
// switched on, 0 for none: no coverage is measured there.
func (m *mutation) runEnv(env []string, id int) []string {
	env = withoutEnv(env, protocol.EnvMutantOverlay, protocol.EnvSuiteCoverage, mutate.EnvMutant, "GOCOVERDIR")
	env = append(env, protocol.EnvMutantOverlay+"="+m.overlay)
	if id > 0 {
		env = append(env, fmt.Sprintf("%s=%d", mutate.EnvMutant, id))
	}
	return env
}

// coverageEnv is the environment of the coverage run, of the tree as it
// is: each suite's coverage kept apart.
func (m *mutation) coverageEnv(env []string) []string {
	env = withoutEnv(env, protocol.EnvMutantOverlay, protocol.EnvSuiteCoverage, mutate.EnvMutant)
	return append(env, protocol.EnvSuiteCoverage+"="+m.suiteDir())
}

// withoutEnv is env without the variables names.
func withoutEnv(env []string, names ...string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(names, name)
	})
}
