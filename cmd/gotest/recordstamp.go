package main

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

// runMode is the mode the run's verdicts are taken under.
func runMode(cfg *ExecConfig) protocol.VerdictMode {
	return protocol.VerdictMode{Policy: cfg.CI, Short: gotestrunner.ShortMode(cfg.GoTestArgs)}
}

// stampRecord gives each package of rec the run's mode and marks those
// narrowed reports.
func stampRecord(rec *protocol.RunRecord, mode protocol.VerdictMode, narrowed func(string) bool, unattributed bool) {
	for i := range rec.Packages {
		p := &rec.Packages[i]
		m := mode
		p.Mode = &m
		if narrowed != nil && narrowed(p.Package) {
			p.Narrowed = true
		}
	}
}

// heldCurrent reports whether --changed holds r as current for a package
// whose hash is hash under a run of mode, and whether only the mode keeps
// it from being so. An entry the record lacks is the zero value: never held.
func heldCurrent(r protocol.PackageVerdict, hash string, mode protocol.VerdictMode) (held, otherMode bool) {
	unchanged := r.InputHash != "" && r.InputHash == hash && !r.Narrowed
	held = unchanged && r.Mode.Covers(mode)
	return held, unchanged && !held
}
