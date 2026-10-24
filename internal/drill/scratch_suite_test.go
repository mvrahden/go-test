package drill_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScratchTestSuite: a drill builds at the same paths on every run of one
// checkout and shard, and only one drill at a time holds them.
//
// Sequential: a child forked meanwhile holds a lock until it execs.
type ScratchTestSuite struct{}

func (s *ScratchTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *ScratchTestSuite) TestTwoDrillsOnOneCheckoutUseTheSamePaths(t *gotest.T) {
	root, checkout := t.TempDir(), t.TempDir()
	first, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	leftover := filepath.Join(first.Dir, "snapshot")
	gotest.NoError(t, os.Mkdir(leftover, 0o700))
	// Released without clearing, as by a drill killed outright.
	gotest.NoError(t, first.Release())

	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(checkout, link))
	second, err := drill.OpenScratch(root, link, 1, 2)
	gotest.NoError(t, err)
	gotest.Equal(t, first.Dir, second.Dir)
	entries, err := os.ReadDir(second.Dir)
	gotest.NoError(t, err)
	gotest.Empty(t, entries, "what the earlier drill left is cleared")
	gotest.NoError(t, second.Clear())
	gotest.NoError(t, second.Release())

	other, err := drill.OpenScratch(root, t.TempDir(), 1, 2)
	gotest.NoError(t, err)
	gotest.NotEqual(t, first.Dir, other.Dir)
	gotest.NoError(t, other.Release())
}

func (s *ScratchTestSuite) TestASecondDrillOnTheSameCheckoutAndShardIsRefused(t *gotest.T) {
	root, checkout := t.TempDir(), t.TempDir()
	held, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	_, err = drill.OpenScratch(root, checkout, 1, 2)
	gotest.ErrorContains(t, err, "another drill is running on")
	sibling, err := drill.OpenScratch(root, checkout, 2, 2)
	gotest.NoError(t, err, "another shard has a scratch of its own")
	gotest.NotEqual(t, held.Dir, sibling.Dir)
	gotest.NoError(t, sibling.Release())
	gotest.NoError(t, held.Release())
	again, err := drill.OpenScratch(root, checkout, 1, 2)
	gotest.NoError(t, err)
	gotest.NoError(t, again.Release())
}

func (s *ScratchTestSuite) TestTheDrillRefusesACheckoutAnotherDrillHolds(t *gotest.T) {
	cache, checkout := t.TempDir(), t.TempDir()
	held, err := drill.OpenScratch(filepath.Join(cache, "gotest", "drill"), checkout, 1, 1)
	gotest.NoError(t, err)
	cmd := childCommand("drill", "--root", checkout)
	cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+cache, "DRILL_SHARD=1/1", "DRILL_LOGS="+t.TempDir())
	out, _ := cmd.CombinedOutput()
	gotest.NoError(t, held.Release())
	gotest.Equal(t, 1, cmd.ProcessState.ExitCode())
	gotest.Regexp(t, `^DRILL  another drill is running on .* as shard 1/1 `, string(out))
}

// own is names whose preferred slot is slot, in order.
func own(names []string, slot, slots int) []string {
	var mine []string
	for _, name := range names {
		if drill.Slot(name, slots) == slot {
			mine = append(mine, name)
		}
	}
	return mine
}

func (s *ScratchTestSuite) TestAMutantKeepsItsSlotWhateverElseIsSelected(t *gotest.T) {
	all := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	for _, names := range [][]string{all, {"c", "h", "a"}, {"j", "e", "a", "b"}} {
		for slot := range 3 {
			mine := own(names, slot, 3)
			q := drill.NewQueues(names, 3)
			var got []string
			for range mine {
				name, _ := q.Next(slot)
				got = append(got, name)
			}
			gotest.Equal(t, mine, got, "slot %d among %v", slot, names)
		}
	}
	gotest.NotEqual(t, own(all, 0, 3), all, "the names spread over the slots")
}

func (s *ScratchTestSuite) TestAnIdleSlotTakesFromTheFullestQueue(t *gotest.T) {
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	q := drill.NewQueues(names, 3)
	for range own(names, 0, 3) {
		_, _ = q.Next(0)
	}
	fullest := own(names, 1, 3)
	if other := own(names, 2, 3); len(other) > len(fullest) {
		fullest = other
	}
	name, ok := q.Next(0)
	gotest.True(t, ok)
	gotest.Equal(t, fullest[0], name)
	var got []string
	for ok {
		got = append(got, name)
		name, ok = q.Next(0)
	}
	gotest.Len(t, got, len(names)-len(own(names, 0, 3)), "one slot alone judges what is left, each once")
	gotest.False(t, slices.Contains(got, ""))
}
