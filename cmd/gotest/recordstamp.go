package main

import (
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

// runMode is the mode the run's verdicts are taken under.
func runMode(cfg *ExecConfig) protocol.VerdictMode {
	return protocol.VerdictMode{Policy: cfg.CI, Short: gotestrunner.ShortMode(cfg.GoTestArgs)}
}

// stampRecord gives each package of rec the run's mode, fails those a
// failure of the run names and marks those narrowed reports. A failure no
// package explains, unattributed or an exit code without a failed package,
// marks every pass: none of them is a verdict of the whole run.
func stampRecord(rec *protocol.RunRecord, mode protocol.VerdictMode, narrowed func(string) bool, unattributed bool) {
	failed := map[string]bool{}
	for _, u := range rec.Failed {
		failed[u.Package] = true
	}
	explained := len(rec.Failed) > 0
	for i := range rec.Packages {
		p := &rec.Packages[i]
		if failed[p.Package] {
			p.Verdict = "fail"
		}
		explained = explained || p.Verdict == "fail"
		m := mode
		p.Mode = &m
		if narrowed != nil && narrowed(p.Package) {
			p.Narrowed = true
		}
	}
	if unattributed || rec.Result.ExitCode != 0 && !explained {
		for i := range rec.Packages {
			if rec.Packages[i].Verdict == "pass" {
				rec.Packages[i].Narrowed = true
			}
		}
	}
}

// declaringPackages are the packages of loaded declaring a test method or
// fuzz target an unfocused run runs, excluded ones aside; nil when the
// suites cannot be read.
func declaringPackages(loaded []*gotestgen.LoadResult) map[string]bool {
	out := map[string]bool{}
	for _, lr := range loaded {
		suites, err := gotestgen.CollectFromLoaded([]*gotestgen.LoadResult{lr})
		if err != nil {
			return nil
		}
		effective, _, _ := suites.ReduceInRun(false)
		for _, s := range effective {
			if len(s.TestCases())+len(s.Fuzzers()) > 0 {
				out[lr.PkgPath] = true
			}
		}
	}
	return out
}

// heldCurrent reports whether --changed holds r as current for a package
// whose hash is hash under a run of mode, and whether only the mode keeps
// it from being so. An entry the record lacks is the zero value: never held.
func heldCurrent(r protocol.PackageVerdict, hash string, mode protocol.VerdictMode) (held, otherMode bool) {
	unchanged := r.InputHash != "" && r.InputHash == hash && !r.Narrowed
	held = unchanged && r.Mode.Covers(mode)
	return held, unchanged && !held
}
