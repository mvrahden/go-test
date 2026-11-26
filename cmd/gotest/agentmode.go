package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/agentenv"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

// agentOutput reports whether a text run prints for a coding agent, and
// what turned it on: --agent, else GOTEST_AGENT, else the variable an agent
// set. --agent=false and GOTEST_AGENT=0 turn it off whatever is detected.
func agentOutput(own OwnFlags, getenv func(string) string) (on bool, why string) {
	if own.Given("--agent") {
		return own.Bool("--agent"), "--agent"
	}
	if v := getenv(protocol.EnvAgentOutput); v != "" {
		return protocol.SwitchOn(v), protocol.EnvAgentOutput + "=" + v
	}
	why = agentenv.Cause(getenv)
	return why != "", why
}

// announceAgentOutput says on stderr that agent output is on, why, and how
// to turn it off.
func announceAgentOutput(w io.Writer, why string) {
	fmt.Fprintf(w, "gotest: agent output on (%s): failures only, the verdict on the last line of stdout; --agent=false or %s=0 turns it off\n", why, protocol.EnvAgentOutput)
}

// safeWord is a word a shell takes as written.
var safeWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes word for a POSIX shell where it needs it.
func shellQuote(word string) string {
	if safeWord.MatchString(word) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// rerunCommand is the command that reruns a run's failures from dir: the
// way the module runs gotest, --failed, then the run's flags without its
// packages and -run, which --failed replaces.
func rerunCommand(dir string, args []string) string {
	prefix := "gotest"
	if mod := findModule(dir); mod != nil {
		prefix = mod.command(dir)
	}
	words := []string{prefix, "--failed"}
	patterns := gotestrunner.PatternIndices(args, ownFlag)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-args" || arg == "--":
			for _, rest := range args[i:] {
				words = append(words, shellQuote(rest))
			}
			return strings.Join(words, " ")
		case slices.Contains(patterns, i), arg == "--failed", strings.HasPrefix(arg, "--failed="), strings.HasPrefix(arg, "-run="):
			continue
		case arg == "-run":
			i++
			continue
		}
		words = append(words, shellQuote(arg))
	}
	return strings.Join(words, " ")
}

// failedSelection reads the record of the last run in dir for --failed: the
// per-package -run that reruns its failures, and their packages.
func failedSelection(dir string) (map[string]string, []string, error) {
	rec, err := gotestrunner.ReadRunRecord(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("--failed: no run is recorded for %s, so nothing is known to have failed; run the tests first", dir)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("--failed: reading the last run's record: %w", err)
	}
	if len(rec.Failed) == 0 {
		return nil, nil, fmt.Errorf("--failed: the last run in %s (gotest %s) exited %d and failed no test, so there is nothing to rerun; run the packages themselves to run everything", dir, strings.Join(rec.Manifest.Args, " "), rec.Result.ExitCode)
	}
	only := gotestrunner.RerunSelection(rec.Failed)
	return only, slices.Sorted(maps.Keys(only)), nil
}

// loadReportPackages loads the packages spec or summary runs, saying on
// stderr why it cannot, as when --failed leaves none of them to rerun.
func loadReportPackages(cfg ExecConfig, goTestArgs []string) (gotestgen.RunPackages, bool) { //nolint:gocritic // hugeParam: once per run
	loadFlags := gotestrunner.StripNonLoadFlags(gotestrunner.ClassifyGoTestArgs(goTestArgs).BuildFlags)
	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return pkgs, false
	}
	return pkgs, !refuseUnfailed(cfg.Only, pkgs.Order)
}

// refuseUnfailed reports, on stderr, a --failed run none of whose packages
// failed in the last run: it would run nothing and pass.
func refuseUnfailed(only map[string]string, pkgs []string) bool {
	if only == nil || slices.ContainsFunc(pkgs, func(p string) bool { _, ok := only[p]; return ok }) {
		return false
	}
	fmt.Fprintln(os.Stderr, "FAIL: --failed: none of the packages given failed in the last run, so there is nothing to rerun")
	return true
}

// recordRun leaves the run's record for --failed in the cache; a run that
// judged nothing leaves none. It returns the failed units.
func recordRun(result *gotestrunner.PipelineResult, code int, manifest *protocol.Manifest) []protocol.FailedUnit {
	if result == nil || result.Judged == nil || manifest == nil {
		return nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return result.Failed
	}
	res := *result.Judged
	res.ExitCode = code
	if err := gotestrunner.WriteRunRecord(protocol.RunRecord{Dir: dir, Manifest: *manifest, Result: res, Failed: result.Failed}); err != nil {
		fmt.Fprintf(os.Stderr, "gotest: the run is not recorded for --failed: %s\n", err)
	}
	return result.Failed
}
