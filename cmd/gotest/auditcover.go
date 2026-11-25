package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
// other counter modes never meet.
func foldChildCoverage(dir, profile string, pkgs []string) (int, error) {
	metas, err := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	if err != nil || len(metas) == 0 {
		return 0, err
	}
	slices.Sort(metas)
	out, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the profile the run wrote
	if err != nil {
		return 0, err
	}
	defer out.Close()
	n := 0
	for i, meta := range metas {
		hash := strings.TrimPrefix(filepath.Base(meta), "covmeta.")
		one := filepath.Join(dir, fmt.Sprintf("binary-%d", i))
		if err := os.Mkdir(one, 0o700); err != nil {
			return 0, err
		}
		counters, err := filepath.Glob(filepath.Join(dir, "covcounters."+hash+".*"))
		if err != nil {
			return 0, err
		}
		if len(counters) == 0 {
			continue
		}
		n++
		for _, f := range append(counters, meta) {
			if err := os.Rename(f, filepath.Join(one, filepath.Base(f))); err != nil {
				return 0, err
			}
		}
		text := filepath.Join(dir, fmt.Sprintf("binary-%d.out", i))
		cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+one, "-o="+text, "-pkg="+strings.Join(pkgs, ",")) //nolint:gosec // G204: go tool with controlled arguments
		if msg, err := cmd.CombinedOutput(); err != nil {
			return 0, fmt.Errorf("go tool covdata: %w: %s", err, strings.TrimSpace(string(msg)))
		}
		data, err := os.ReadFile(text) //nolint:gosec // G304: the file covdata just wrote
		if err != nil {
			return 0, err
		}
		if _, err := out.Write(data); err != nil {
			return 0, err
		}
	}
	return n, out.Close()
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
