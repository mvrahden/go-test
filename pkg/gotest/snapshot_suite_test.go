package gotest_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SnapshotTestSuite struct{}

func (s *SnapshotTestSuite) TestMatchSnapshot(t *gotest.T) {
	snapDir := filepath.Join("testdata", "__snapshots__")
	t.T().Cleanup(func() { os.RemoveAll(snapDir) })

	t.When("no snapshot exists", func(w *gotest.T) {
		w.It("creates the snapshot on first run", func(it *gotest.T) {
			it.MatchSnapshot("hello world")

			snapName := sanitizeForTest(it.T().Name()) + ".snap"
			data, err := os.ReadFile(filepath.Join(snapDir, snapName))
			gotest.NoError(it, err)
			gotest.Equal(it, "hello world", string(data))
		})
	})

	t.When("snapshot already exists", func(w *gotest.T) {
		w.It("matches without error", func(it *gotest.T) {
			it.MatchSnapshot("stable value")
			it.MatchSnapshot("stable value")
		})
	})

	t.When("custom name is provided", func(w *gotest.T) {
		w.It("uses the custom name in the filename", func(it *gotest.T) {
			it.MatchSnapshot("custom content", "my-snapshot")

			snapName := sanitizeForTest(it.T().Name()) + "_my-snapshot.snap"
			data, err := os.ReadFile(filepath.Join(snapDir, snapName))
			gotest.NoError(it, err)
			gotest.Equal(it, "custom content", string(data))
		})
	})

	t.When("update mode is enabled", func(w *gotest.T) {
		w.It("overwrites the existing snapshot", func(it *gotest.T) {
			it.MatchSnapshot("original")

			it.T().Setenv("GOTEST_UPDATE_SNAPSHOTS", "1")
			it.MatchSnapshot("updated")

			snapName := sanitizeForTest(it.T().Name()) + ".snap"
			data, err := os.ReadFile(filepath.Join(snapDir, snapName))
			gotest.NoError(it, err)
			gotest.Equal(it, "updated", string(data))
		})
	})
}
