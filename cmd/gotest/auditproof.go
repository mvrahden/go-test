package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// prove adds to the document what the runs and the coverage given say of
// the change: the evidence of the streams given and of the run --run makes,
// and the changed production lines no test executed.
func (a *auditArgs) prove(doc *gotestaudit.Document, base, head *gotestaudit.Record, git *gitBase) error {
	if len(a.evidence)+len(a.baseEvidence) == 0 && !a.run && a.coverage == "" {
		return nil
	}
	var files []gotestaudit.ProdFile
	var lines string
	if a.unproven() {
		var err error
		if files, lines, err = a.changedFiles(git); err != nil {
			return err
		}
	}
	var inputs []gotestaudit.StreamInput
	profile := a.coverage
	var mut *mutation
	if a.mutate {
		var err error
		if mut, err = a.prepareMutation(doc, files); err != nil {
			return err
		}
		defer mut.release()
		if mut != nil {
			if err := mut.testPackages(a); err != nil {
				return err
			}
		}
	}
	var zero *baseline
	if a.run {
		in, ranProfile, err := a.runChanged(doc, files, mut)
		if err != nil {
			return err
		}
		if in != nil {
			inputs = append(inputs, *in)
		}
		defer removeProfile(ranProfile)
		if profile == "" {
			profile = ranProfile
		}
		if mut != nil && in != nil {
			if zero, err = a.readMutationBaseline(doc, mut, in.Events, ranProfile); err != nil {
				return err
			}
		}
	}
	for _, side := range []struct {
		name  string
		files []string
	}{{gotestaudit.SideHead, a.evidence}, {gotestaudit.SideBase, a.baseEvidence}} {
		for _, file := range side.files {
			events, err := readStreamFile(file)
			if err != nil {
				return fmt.Errorf("--%s: %w", evidenceFlag(side.name), err)
			}
			in := gotestaudit.StreamInput{File: file, Side: side.name, Events: events}
			if side.name == gotestaudit.SideHead && a.headFile == "" {
				if in.Fresh, err = freshOnDisk(events); err != nil {
					return fmt.Errorf("--evidence: %w", err)
				}
			}
			inputs = append(inputs, in)
		}
	}
	if len(inputs) > 0 {
		gotestaudit.AddEvidence(doc, base, head, inputs)
	}
	if a.unproven() && profile != "" {
		// The run's own profile may hold no block: then no file has data.
		blocks, err := readProfile(profile, a.coverage == "")
		if err != nil {
			return fmt.Errorf("--coverage: %w", err)
		}
		shown := a.coverage
		gotestaudit.AddUnproven(doc, files, blocks, gotestaudit.Coverage{Profile: shown, Lines: lines})
	}
	if mut != nil {
		a.judge(doc, mut, zero)
	}
	return nil
}

// freshOnDisk says, of each package whose verdict events stamped with an
// input hash, whether the working tree's package still has it.
func freshOnDisk(events []gotestspec.TestEvent) (map[string]bool, error) {
	stamped := map[string]string{}
	var pkgs []string
	for i := range events {
		ev := &events[i]
		if ev.InputHash == "" || ev.RunLevel || ev.Test != "" {
			continue
		}
		if _, ok := stamped[ev.Package]; !ok {
			pkgs = append(pkgs, ev.Package)
		}
		stamped[ev.Package] = ev.InputHash
	}
	if len(pkgs) == 0 {
		return nil, nil
	}
	current, err := currentHashes(context.Background(), pkgs, streamBuild(events))
	if err != nil {
		return nil, err
	}
	fresh := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		fresh[pkg] = current[pkg] == stamped[pkg]
	}
	return fresh, nil
}

func evidenceFlag(side string) string {
	if side == gotestaudit.SideBase {
		return "base-evidence"
	}
	return "evidence"
}

func removeProfile(path string) {
	if path != "" {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}

// readStreamFile reads a -json stream; "-" is stdin.
func readStreamFile(file string) ([]gotestspec.TestEvent, error) {
	var r io.Reader = os.Stdin
	if file != "-" {
		f, err := os.Open(file) //nolint:gosec // G304: the stream the user named
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("%s holds no test event", file)
	}
	return events, nil
}

func readProfile(path string, empty bool) ([]gotestspec.CoverageBlock, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the profile the user named or the run wrote
	if err != nil {
		return nil, err
	}
	defer f.Close()
	blocks, err := gotestspec.ReadCoverageBlocks(f)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 && !empty {
		return nil, fmt.Errorf("%s holds no coverage block", path)
	}
	return blocks, nil
}

// changedFiles are the changed production files of the packages the
// patterns match, with their changed lines, and where the lines came from.
func (a *auditArgs) changedFiles(git *gitBase) ([]gotestaudit.ProdFile, string, error) {
	var changes []gotestaudit.FileLines
	var root, from string
	switch {
	case a.diff != "":
		var r io.Reader = os.Stdin
		if a.diff != "-" {
			f, err := os.Open(a.diff)
			if err != nil {
				return nil, "", fmt.Errorf("--diff: %w", err)
			}
			defer f.Close()
			r = f
		}
		var err error
		if changes, err = gotestaudit.ParseDiff(r); err != nil {
			return nil, "", fmt.Errorf("--diff: %w", err)
		}
		from = "the diff " + a.diff
	default:
		var err error
		if changes, err = changedLines(git); err != nil {
			return nil, "", err
		}
		root = git.top
		from = "git diff " + git.short + " and the working tree, its untracked files included"
	}
	pkgs, _, err := gotestgen.ListPackageDirs("", a.patterns, a.loadFlags)
	if err != nil {
		return nil, "", err
	}
	byDir := map[string]string{}
	for _, p := range pkgs {
		if p.Dir != "" {
			byDir[p.Dir] = p.ImportPath
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	var out []gotestaudit.ProdFile
	for _, c := range changes {
		if !strings.HasSuffix(c.Path, ".go") || strings.HasSuffix(c.Path, "_test.go") || len(c.Lines) == 0 {
			continue
		}
		abs := diffFile(root, cwd, c.Path)
		if abs == "" {
			continue
		}
		pkg, ok := byDir[filepath.Dir(abs)]
		if !ok {
			continue
		}
		modRoot, mod := moduleRootOf(filepath.Dir(abs))
		rel, err := filepath.Rel(modRoot, abs)
		if err != nil {
			continue
		}
		out = append(out, gotestaudit.ProdFile{Module: mod, File: filepath.ToSlash(rel), Package: pkg, Lines: c.Lines, Abs: abs})
	}
	return out, from, nil
}

// diffFile is the file a diff path names: under root when given, else under
// the current directory or the first of its parents that holds it.
func diffFile(root, cwd, name string) string {
	rel := filepath.FromSlash(name)
	if root != "" {
		return filepath.Join(root, rel)
	}
	for dir := cwd; ; {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			return filepath.Join(dir, rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// moduleRootOf is the root of the module enclosing dir and its path.
func moduleRootOf(dir string) (root, path string) {
	for root = dir; ; {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, gotestspec.ReadModulePath(root)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return dir, ""
		}
		root = parent
	}
}

// runChanged runs, through this CLI's runner, the packages holding the
// behaviors the document names and, when the unproven lines are asked for,
// the changed production packages, measuring their coverage. It returns
// the run's stream, nil when there is nothing to run, and its profile.
func (a *auditArgs) runChanged(doc *gotestaudit.Document, files []gotestaudit.ProdFile, mut *mutation) (*gotestaudit.StreamInput, string, error) {
	run, cover := runSet(doc, files, mut)
	if len(run) == 0 {
		doc.Notes = append(doc.Notes, "--run: no package holds a changed behavior or a changed production file, so nothing ran")
		return nil, "", nil
	}
	dir, err := os.MkdirTemp(os.Getenv("GOTMPDIR"), "gotest-audit-run-")
	if err != nil {
		return nil, "", err
	}
	args := append([]string{"-json"}, a.loadFlags...)
	if a.parallel > 0 {
		args = append([]string{fmt.Sprintf("--parallel=%d", a.parallel)}, args...)
	}
	if a.control {
		args = append([]string{"--control=stdin"}, args...)
	}
	profile, covdir := "", ""
	if a.unproven() && len(cover) > 0 {
		profile = filepath.Join(dir, "cover.out")
		args = append(args, "-coverprofile="+profile, "-coverpkg="+strings.Join(cover, ","))
		// The binaries the tests start write their coverage here.
		covdir = filepath.Join(dir, "covdata")
		if err := os.Mkdir(covdir, 0o700); err != nil {
			_ = os.RemoveAll(dir)
			return nil, "", err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", err
	}
	cmd := exec.Command(exe, append(args, run...)...) //nolint:gosec // G204: this CLI, running the changed packages
	if covdir != "" {
		cmd.Env = withCoverDir(os.Environ(), covdir)
	}
	if mut != nil {
		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		cmd.Env = mut.coverageEnv(cmd.Env)
		if err := os.MkdirAll(mut.suiteDir(), 0o700); err != nil {
			_ = os.RemoveAll(dir)
			return nil, "", err
		}
	}
	var stdout strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, os.Stderr
	if a.control {
		cmd.Stdin = os.Stdin
	}
	runErr := runForwarding(cmd)
	if mut != nil {
		mut.exit = exitCodeOf(runErr)
	}
	var exit *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exit) {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("--run: %w", runErr)
	}
	if exit != nil && exit.ExitCode() == 130 {
		a.stopped = true
		doc.Incomplete = append(doc.Incomplete, "--run: the run was stopped before its end; what it did not reach reads as not run")
		doc.Summary.Incomplete = len(doc.Incomplete)
	}
	events, err := gotestspec.ParseEvents(strings.NewReader(stdout.String()))
	if err != nil || len(events) == 0 {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("--run: the run wrote no stream: %v", runErr)
	}
	if covdir != "" && mut == nil {
		switch n, err := foldChildCoverage(covdir, profile, cover); {
		case err != nil:
			doc.Notes = append(doc.Notes, "--run: the coverage data of the binaries the tests started was not read: "+err.Error())
		case n == 0 && !a.stopped:
			doc.Notes = append(doc.Notes, childCoverNote)
		default:
			doc.Notes = append(doc.Notes, fmt.Sprintf("--run: the coverage counts what %s the tests started executed", binaries(n)))
		}
	}
	if profile != "" {
		if _, err := os.Stat(profile); err != nil {
			doc.Notes = append(doc.Notes, "--run: the run wrote no coverage profile")
			profile = ""
		}
	}
	if profile == "" {
		_ = os.RemoveAll(dir)
	} else {
		doc.Notes = append(doc.Notes, "--run: coverage is of the packages the run ran: "+strings.Join(run, " "))
	}
	return &gotestaudit.StreamInput{Side: gotestaudit.SideHead, Ran: true, Exit: exitCodeOf(runErr), Events: events}, profile, nil
}

// readMutationBaseline reads the run --mutate made with no mutant on as the
// baseline, its children's coverage folded into the run's profile.
func (a *auditArgs) readMutationBaseline(doc *gotestaudit.Document, mut *mutation, events []gotestspec.TestEvent, profile string) (*baseline, error) {
	b, n, err := mut.readBaseline(events, mut.exit, profile)
	if err != nil {
		return nil, err
	}
	switch {
	case n == 0 && !a.stopped:
		doc.Notes = append(doc.Notes, childCoverNote)
	case n > 0:
		doc.Notes = append(doc.Notes, fmt.Sprintf("--run: the coverage counts what %s the tests started executed", binaries(n)))
	}
	return b, nil
}

// runSet is what --run runs: the packages of the behaviors the document
// names, the changed production packages and, under --mutate, every one
// holding tests; and the packages it measures coverage of.
func runSet(doc *gotestaudit.Document, files []gotestaudit.ProdFile, mut *mutation) (run, cover []string) {
	for i := range doc.Behaviors {
		if c := &doc.Behaviors[i]; c.Head != nil {
			run = append(run, c.Package)
		}
	}
	for i := range doc.Findings {
		if f := &doc.Findings[i]; f.Package != "" && f.Class != gotestaudit.ClassRemoved {
			run = append(run, f.Package)
		}
	}
	for i := range files {
		cover = append(cover, files[i].Package)
	}
	slices.Sort(cover)
	cover = slices.Compact(cover)
	run = append(run, cover...)
	if mut != nil {
		run = append(run, mut.tests...)
	}
	slices.Sort(run)
	run = slices.Compact(run)
	return run, cover
}
