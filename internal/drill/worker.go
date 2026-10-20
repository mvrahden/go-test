package drill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Spec is one worker's job, written by the drill and read by --worker.
type Spec struct {
	Name     string `json:"name"`
	Work     string `json:"work"`
	Snapshot string `json:"snapshot"`
	Patch    string `json:"patch"`
	Log      string `json:"log"`
	Result   string `json:"result"`
	Runs     []Run  `json:"runs"`
	Judge    string `json:"judge"`
	CPUs     int    `json:"cpus"`
}

// Runner runs a prepared command and returns its exit code, the negated
// signal number when a signal ended it; err only when it could not run.
type Runner func(cmd *exec.Cmd) (int, error)

// RunCommand is the Runner that actually runs the command.
func RunCommand(cmd *exec.Cmd) (int, error) {
	err := cmd.Run()
	if cmd.ProcessState == nil {
		return 0, err
	}
	return exitCode(cmd.ProcessState), nil
}

func exitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return -int(ws.Signal())
	}
	return ps.ExitCode()
}

func itoa(n int) string { return strconv.Itoa(n) }

func now() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// Worker applies one mutant to a fresh copy of the snapshot, judges it with
// the pristine judge and writes its verdict to spec.Result.
func Worker(spec *Spec, run Runner) {
	started := time.Now()
	timings := map[string]float64{}
	tree, tmp := filepath.Join(spec.Work, "tree"), filepath.Join(spec.Work, "tmp")
	result := Verdict{Name: spec.Name}
	env := envList(WorkerEnv(Environ(), spec.Work, tmp, spec.CPUs))
	notePhase := func(label string) {
		data, _ := json.Marshal(map[string]any{"phase": label, "time": now()})
		_ = os.WriteFile(spec.Log+".phase.json", append(data, '\n'), 0o600)
	}
	var log *os.File
	phase := func(label string, stdout io.Writer, name string, args ...string) (int, error) {
		notePhase(label)
		began := time.Now()
		cmd := exec.Command(name, args...) //nolint:gosec // G204: the drill's own phases
		cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = tree, env, stdout, log
		code, err := run(cmd)
		timings[label] += time.Since(began).Seconds()
		return code, err
	}
	judge := func() error {
		notePhase("copy")
		began := time.Now()
		// cache/cli and GOCACHE are keyed on the sources; gotest's own
		// entries in cache/children were written by an earlier mutant's CLI.
		for _, dir := range []string{tree, tmp, filepath.Join(spec.Work, "cache", "children")} {
			_ = removeAll(dir)
		}
		if err := copyTree(spec.Snapshot, tree, nil); err != nil {
			return err
		}
		if err := os.Mkdir(tmp, 0o777); err != nil {
			return err
		}
		timings["copy"] = time.Since(began).Seconds()
		var err error
		if log, err = os.Create(spec.Log); err != nil {
			return err
		}
		if err := judgeMutant(spec, phase, log); err != nil {
			_ = log.Close()
			return err
		}
		if err := log.Close(); err != nil {
			return err
		}
		notePhase("expectations")
		return expectations(spec, run)
	}
	if err := judge(); err != nil {
		result.Reason = err.Error()
	} else {
		result.Caught, result.Reason = true, "caught"
	}
	result.Seconds, result.Phases = time.Since(started).Seconds(), timings
	data, _ := json.Marshal(result)
	_ = os.WriteFile(spec.Result, append(data, '\n'), 0o600)
	notePhase("done")
}

type phaseFunc func(label string, stdout io.Writer, name string, args ...string) (int, error)

func judgeMutant(spec *Spec, phase phaseFunc, log *os.File) error {
	if code, err := phase("patch", log, "patch", "-p1", "-s", "--fuzz=0", "--forward",
		"--no-backup-if-mismatch", "-r", "-", "-i", spec.Patch); err != nil {
		return err
	} else if code != 0 {
		return errors.New("patch does not apply, refresh it")
	}
	// Keep the whole-source build contract; compile generated tests only
	// in the actual judge pipeline, whose build-fail events are checked.
	if code, err := phase("build", log, "go", "build", "./..."); err != nil {
		return err
	} else if code != 0 {
		return errors.New("mutant does not compile, refresh it")
	}
	failed := false
	cpus := strconv.Itoa(spec.CPUs)
	for i, r := range spec.Runs {
		stream := fmt.Sprintf("%s.%d.jsonl", spec.Log, i)
		args := []string{"-json", "--parallel=" + cpus, "--compile-parallel=" + cpus}
		if r.Pattern != "-" {
			args = append(args, "-run", r.Pattern)
		}
		events, err := os.Create(stream)
		if err != nil {
			return err
		}
		code, err := phase("judge", events, spec.Judge, append(args, r.Packages...)...)
		if cerr := events.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if code < 0 || code > 2 {
			return fmt.Errorf("judge terminated abnormally (%d)", code)
		}
		text, err := os.ReadFile(stream)
		if err != nil {
			return err
		}
		if broken, err := BuildFailure(string(text)); err != nil {
			return err
		} else if broken {
			return errors.New("generated judge harness does not compile, refresh it")
		}
		if len(text) == 0 {
			return errors.New("judge emitted no events")
		}
		failed = failed || code != 0
		if code, err := phase("render", log, spec.Judge, "summary", "--input="+stream, "--render-only", "--no-color"); err != nil {
			return err
		} else if code != 0 {
			return errors.New("could not render judge events")
		}
	}
	if !failed {
		return errors.New("SURVIVED, the guards did not catch it")
	}
	return nil
}

// expectations requires every Expect: line of the patch to match the log
// with colors stripped. grep keeps the patches' POSIX ERE meaning.
func expectations(spec *Spec, run Runner) error {
	text, err := os.ReadFile(spec.Log)
	if err != nil {
		return err
	}
	plain := spec.Log + ".plain"
	if err := os.WriteFile(plain, ansi.ReplaceAll(text, nil), 0o600); err != nil {
		return err
	}
	patch, err := os.ReadFile(spec.Patch)
	if err != nil {
		return err
	}
	header, _, _ := strings.Cut(string(patch), "--- ")
	var expects []string
	for _, line := range lines(header) {
		if expression, ok := strings.CutPrefix(line, "Expect: "); ok {
			expects = append(expects, expression)
		}
	}
	if len(expects) == 0 {
		return errors.New("no Expect: line")
	}
	for _, expression := range expects {
		cmd := exec.Command("grep", "-qE", "--", expression, plain) //nolint:gosec // G204: the patch's own expression
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if code, err := run(cmd); err != nil {
			return err
		} else if code != 0 {
			return errors.New("caught by the wrong check; no line matches: " + expression)
		}
	}
	return nil
}

// copyTree copies src to dst, which must not exist, following symlinks and
// keeping modes and modification times. skip names entries left out.
func copyTree(src, dst string, skip func(path string, d fs.DirEntry) bool) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.Mkdir(dst, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		from, to := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if skip != nil && skip(from, e) {
			continue
		}
		target, err := os.Stat(from)
		if err != nil {
			return err
		}
		switch {
		case target.IsDir():
			err = copyTree(from, to, skip)
		case target.Mode().IsRegular():
			err = copyFile(from, to, target)
		default:
			err = fmt.Errorf("%s is not a regular file", from)
		}
		if err != nil {
			return err
		}
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func copyFile(src, dst string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// removeAll removes path, first making read-only directories writable.
func removeAll(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	makeWritable(path)
	return os.RemoveAll(path)
}

// makeWritable gives the owner full access to dir and the directories below it.
func makeWritable(dir string) {
	_ = os.Chmod(dir, 0o700)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			makeWritable(filepath.Join(dir, e.Name()))
		}
	}
}
