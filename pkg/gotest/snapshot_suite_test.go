package gotest_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SnapshotTestSuite struct{}

func (s *SnapshotTestSuite) TestMatchSnapshot(t *gotest.T) {
	snapDir := filepath.Join("testdata", "__snapshots__")
	t.T().Cleanup(func() { os.RemoveAll(snapDir) })

	t.When("no snapshot exists", func(w *gotest.T) {
		w.It("creates a grouped snapshot file on first run", func(it *gotest.T) {
			gotest.MatchSnapshot(it,"hello world")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.True(it, strings.Contains(string(data), "hello world"))
		})
	})

	t.When("snapshot already exists", func(w *gotest.T) {
		w.It("matches multiple snapshots with dedup suffixes", func(it *gotest.T) {
			gotest.MatchSnapshot(it,"stable value")
			gotest.MatchSnapshot(it,"stable value")
		})
	})

	t.When("custom name is provided", func(w *gotest.T) {
		w.It("uses the custom name in the section key", func(it *gotest.T) {
			gotest.MatchSnapshot(it,"custom content", "my-snapshot")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.True(it, strings.Contains(string(data), "my-snapshot"))
		})
	})

	t.When("update mode is enabled", func(w *gotest.T) {
		w.It("overwrites the existing snapshot", func(it *gotest.T) {
			gotest.MatchSnapshot(it,"original")

			it.T().Setenv("GOTEST_UPDATE_SNAPSHOTS", "1")
			gotest.MatchSnapshot(it,"updated")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.True(it, strings.Contains(content, "updated"))
		})
	})
}
