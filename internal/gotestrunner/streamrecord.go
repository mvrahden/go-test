package gotestrunner

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// RecordOfStream is the record a saved -json stream holds: its run's
// manifest and close where it has them, each package's last verdict with
// its input hash and time, and the units it failed. A stream without a
// close is judged from its verdicts: exit 1 when one failed, else 0.
func RecordOfStream(stream []byte) (protocol.RunRecord, error) {
	var rec protocol.RunRecord
	tally := &runTally{packages: map[string]string{}, elapsed: map[string]float64{}, hashes: map[string]string{}}
	runLevel := map[string]bool{}
	var closed *protocol.RunResult
	for line := range bytes.Lines(stream) {
		var ev struct {
			Time      time.Time
			Action    string
			Package   string
			Test      string
			Elapsed   float64
			InputHash string
			RunLevel  bool
			Manifest  *protocol.Manifest
			Result    *protocol.RunResult
		}
		if json.Unmarshal(bytes.TrimSpace(line), &ev) != nil || ev.Package == "" {
			continue
		}
		if rec.Started.IsZero() && !ev.Time.IsZero() {
			rec.Started = ev.Time.UTC()
		}
		if ev.RunLevel {
			runLevel[ev.Package] = true
		}
		if ev.Package == protocol.RunPackage {
			if ev.Manifest != nil {
				rec.Manifest = *ev.Manifest
			}
			if ev.Result != nil {
				closed = ev.Result
			}
			continue
		}
		tally.count(ev.Package, ev.Test, ev.Action, ev.Elapsed, ev.InputHash)
	}
	rec.Packages = tally.verdicts(runLevel)
	if len(rec.Packages) == 0 {
		return rec, errors.New("the stream holds no package's verdict")
	}
	res, failed := tally.snapshot(runLevel)
	rec.Failed = narrowFailures(failed)
	if closed != nil {
		rec.Result = *closed
	} else {
		rec.Result = res
		if res.Packages.Failed > 0 || res.Tests != nil && res.Tests.Failed > 0 {
			rec.Result.ExitCode = 1
		}
	}
	return rec, nil
}
