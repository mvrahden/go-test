package gotestrunner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// A checkout's runs live in runs/<key>/ under the gotest cache: last.json,
// the record --failed and --changed read, and history/, one file per run,
// newest last by name. Writers take runs/<key>/lock, so runs side by side
// in one checkout merge into last.json one at a time; each file is written
// by a rename, so a reader never sees half of one.

// Bounds of a checkout's history, applied on each write.
const (
	HistoryCount = 50
	HistoryAge   = 30 * 24 * time.Hour
)

// historyDir is where the runs made in dir keep their records.
func historyDir(root, dir string) string {
	return filepath.Join(filepath.Dir(recordPathIn(root, dir)), "history")
}

// SaveRun records rec as the last run made in rec.Dir and keeps it in the
// checkout's history. With merge, last.json keeps the packages rec did not
// run, and their failed units, from the record before it. now dates it.
func SaveRun(rec protocol.RunRecord, merge bool, now time.Time) error { //nolint:gocritic // hugeParam: once per run
	root, err := cacheRoot()
	if err != nil {
		return err
	}
	return saveRunIn(root, rec, merge, now)
}

func saveRunIn(root string, rec protocol.RunRecord, merge bool, now time.Time) error { //nolint:gocritic // hugeParam: once per run
	rec.Time = now.UTC()
	unlock, err := lockRuns(root, rec.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	last := rec
	if merge {
		if before, err := readRecordIn(root, rec.Dir); err == nil {
			last = mergeRecords(before, rec)
		}
	}
	if err := writeRecordIn(root, last); err != nil {
		return err
	}
	return keep(root, rec, now)
}

// ImportRun keeps rec, read from a saved stream, in its checkout's
// history; last.json is left as it is.
func ImportRun(rec protocol.RunRecord, now time.Time) error { //nolint:gocritic // hugeParam: once per import
	root, err := cacheRoot()
	if err != nil {
		return err
	}
	rec.Imported = true
	if rec.Time.IsZero() {
		rec.Time = now.UTC()
	}
	unlock, err := lockRuns(root, rec.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	return keep(root, rec, now)
}

// lockRuns takes the checkout's lock of its run records.
func lockRuns(root, dir string) (func(), error) {
	base := filepath.Dir(recordPathIn(root, dir))
	for {
		// A clean may remove the directory while a run waits for it.
		if err := os.MkdirAll(base, 0o755); err != nil {
			return nil, err
		}
		unlock, err := lockFile(filepath.Join(base, "lock"))
		if errors.Is(err, errLockReplaced) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		return unlock, err
	}
}

// mergeRecords is next over before: next's run, before's packages and
// failed units of the packages next did not run.
func mergeRecords(before, next protocol.RunRecord) protocol.RunRecord { //nolint:gocritic // hugeParam: once per run
	ran := map[string]bool{}
	for _, p := range next.Packages {
		ran[p.Package] = true
	}
	for _, u := range next.Failed {
		ran[u.Package] = true
	}
	out := next
	out.Packages = nil
	for _, p := range before.Packages {
		if !ran[p.Package] {
			out.Packages = append(out.Packages, p)
		}
	}
	out.Packages = append(out.Packages, next.Packages...)
	out.Failed = nil
	for _, u := range before.Failed {
		if !ran[u.Package] {
			out.Failed = append(out.Failed, u)
		}
	}
	out.Failed = append(out.Failed, next.Failed...)
	return out
}

// keep writes rec into the history and prunes it: all but the newest
// HistoryCount, and those older than HistoryAge.
func keep(root string, rec protocol.RunRecord, now time.Time) error { //nolint:gocritic // hugeParam: once per run
	dir := historyDir(root, rec.Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	var salt [4]byte
	_, _ = rand.Read(salt[:])
	name := fmt.Sprintf("%s-%d-%s.json", now.UTC().Format("20060102T150405.000000000Z"), os.Getpid(), hex.EncodeToString(salt[:]))
	if err := writeAtomic(filepath.Join(dir, name), append(data, '\n')); err != nil {
		return err
	}
	prune(dir, now)
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil && cerr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	return nil
}

// prune removes the records past the history's bounds, by the time each
// was written, which its name begins with.
func prune(dir string, now time.Time) {
	names := historyNames(dir)
	for i, name := range names {
		at, ok := recordTime(name)
		if i < len(names)-HistoryCount || ok && now.Sub(at) > HistoryAge {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// historyNames are the records of a history, oldest first.
func historyNames(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}

func recordTime(name string) (time.Time, bool) {
	stamp, _, ok := strings.Cut(name, "-")
	if !ok {
		return time.Time{}, false
	}
	at, err := time.Parse("20060102T150405.000000000Z", stamp)
	return at, err == nil
}

// KeptRun is one record of a checkout's history.
type KeptRun struct {
	// ID names the record among the checkout's: its file's name.
	ID     string
	Record protocol.RunRecord
}

// KeptRuns are the records the runs made in dir kept, newest first; a
// file that does not parse is left out.
func KeptRuns(dir string) ([]KeptRun, error) {
	root, err := cacheRoot()
	if err != nil {
		return nil, err
	}
	return keptRunsIn(root, dir), nil
}

func keptRunsIn(root, dir string) []KeptRun {
	hdir := historyDir(root, dir)
	names := historyNames(hdir)
	out := make([]KeptRun, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		data, err := os.ReadFile(filepath.Join(hdir, names[i])) //nolint:gosec // G304: a record under gotest's own cache
		if err != nil {
			continue
		}
		var rec protocol.RunRecord
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		out = append(out, KeptRun{ID: strings.TrimSuffix(names[i], ".json"), Record: rec})
	}
	return out
}

// ClearRuns removes the run records of every checkout whose lock no run
// holds: last.json and the history. It returns how many checkouts' records
// it removed and how many it left to a run writing them.
func ClearRuns() (removed, inUse int, dir string, err error) {
	root, err := cacheRoot()
	if err != nil {
		return 0, 0, "", err
	}
	removed, inUse = clearRunsIn(root)
	return removed, inUse, filepath.Join(root, "runs"), nil
}

func clearRunsIn(root string) (removed, inUse int) {
	runs := filepath.Join(root, "runs")
	entries, _ := os.ReadDir(runs)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		base := filepath.Join(runs, e.Name())
		lock := filepath.Join(base, "lock")
		unlock, err := tryLockFile(lock)
		if err != nil {
			inUse++
			continue
		}
		_ = os.RemoveAll(filepath.Join(base, "history"))
		_ = os.Remove(filepath.Join(base, "last.json"))
		removed++
		releaseEvicted(lock, unlock)
		_ = os.Remove(base)
	}
	return removed, inUse
}
