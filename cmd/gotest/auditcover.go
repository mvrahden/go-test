package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/coverage"
	"slices"
	"strings"
)

// childCoverNote is the document's note when no binary a test started
// wrote coverage data, naming how one does.
const childCoverNote = "--run: no binary a test started wrote coverage data; one counts when built with go build -cover (https://github.com/mvrahden/go-test/blob/main/specs/audit/spec.md#requirement-coverage-of-the-binaries-a-test-starts)"

// withCoverDir is env with GOCOVERDIR set to dir, the one the caller's
// environment may name dropped.
func withCoverDir(env []string, dir string) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, "GOCOVERDIR=") })
	return append(env, "GOCOVERDIR="+dir)
}

// foldChildCoverage appends to profile, as its text format, the coverage
// data the binaries the run's tests started wrote into dir, limited to
// pkgs. It returns how many binaries wrote counters; one that never exited
// wrote none. Each binary's data is converted alone, so binaries built with
// other counter modes never meet. The run's own gotest, this binary, is no
// binary a test started: built with -cover it writes there too, and its
// data is dropped.
func foldChildCoverage(dir, profile string, pkgs []string) (int, error) {
	text, n, err := childCoverage(dir, pkgs)
	if err != nil || n == 0 {
		return 0, err
	}
	return n, appendFile(profile, text)
}

// appendFile appends data to the file at path.
func appendFile(path string, data []byte) error {
	out, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: a profile the run wrote
	if err != nil {
		return err
	}
	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// childCoverage is the coverage data binaries wrote into dir as a profile's
// text, limited to pkgs, and how many binaries wrote counters.
func childCoverage(dir string, pkgs []string) ([]byte, int, error) {
	metas, err := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	if err != nil || len(metas) == 0 {
		return nil, 0, err
	}
	if self := ownMeta(filepath.Join(filepath.Dir(dir), "covself")); self != "" {
		metas = slices.DeleteFunc(metas, func(m string) bool { return filepath.Base(m) == self })
	}
	slices.Sort(metas)
	var out []byte
	n := 0
	for i, meta := range metas {
		hash := strings.TrimPrefix(filepath.Base(meta), "covmeta.")
		one := filepath.Join(dir, fmt.Sprintf("binary-%d", i))
		if err := os.Mkdir(one, 0o700); err != nil {
			return nil, 0, err
		}
		counters, err := filepath.Glob(filepath.Join(dir, "covcounters."+hash+".*"))
		if err != nil {
			return nil, 0, err
		}
		if len(counters) == 0 {
			continue
		}
		n++
		for _, f := range append(counters, meta) {
			if err := os.Rename(f, filepath.Join(one, filepath.Base(f))); err != nil {
				return nil, 0, err
			}
		}
		text := filepath.Join(dir, fmt.Sprintf("binary-%d.out", i))
		cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+one, "-o="+text, "-pkg="+strings.Join(pkgs, ",")) //nolint:gosec // G204: go tool with controlled arguments
		if msg, err := cmd.CombinedOutput(); err != nil {
			return nil, 0, fmt.Errorf("go tool covdata: %w: %s", err, strings.TrimSpace(string(msg)))
		}
		data, err := os.ReadFile(text) //nolint:gosec // G304: the file covdata just wrote
		if err != nil {
			return nil, 0, err
		}
		out = append(out, data...)
	}
	return out, n, nil
}

// ownMeta is the name of this binary's coverage meta-data file, written
// into dir, or "" when it was built without -cover.
func ownMeta(dir string) string {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return ""
	}
	if err := coverage.WriteMetaDir(dir); err != nil {
		return ""
	}
	metas, _ := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	if len(metas) != 1 {
		return ""
	}
	return filepath.Base(metas[0])
}

// runForwarding runs cmd, passing it each shutdown signal the audit gets,
// so the run stops as one the terminal stopped and the audit outlives it to
// write its document and remove the run's directory. A signal the child got
// from the terminal too reaches it twice, which its runner reads as one.
// Windows delivers the console's events to both; there the forward fails
// and is dropped.
func runForwarding(cmd *exec.Cmd) error {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, shutdownSignals...)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-sigs:
				_ = cmd.Process.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	return cmd.Wait()
}

func binaries(n int) string {
	if n == 1 {
		return "1 binary"
	}
	return fmt.Sprintf("%d binaries", n)
}
