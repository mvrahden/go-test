package gotest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func snapshotDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "__snapshots__")
}

func TestMatchSnapshot_CreatesGroupedFile(t *testing.T) {
	dir := snapshotDir()
	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Run("sub1", func(t *testing.T) {
		gotest.MatchSnapshot(t, "value-one")
	})
	t.Run("sub2", func(t *testing.T) {
		gotest.MatchSnapshot(t, "value-two")
	})

	snapPath := filepath.Join(dir, "TestMatchSnapshot_CreatesGroupedFile.snap")
	data, err := os.ReadFile(snapPath)
	gotest.NoError(t, err)

	content := string(data)
	gotest.True(t, strings.Contains(content, "=== SNAP sub1 ==="))
	gotest.True(t, strings.Contains(content, "value-one"))
	gotest.True(t, strings.Contains(content, "=== SNAP sub2 ==="))
	gotest.True(t, strings.Contains(content, "value-two"))
}

func TestMatchSnapshot_CustomName(t *testing.T) {
	dir := snapshotDir()
	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Run("case", func(t *testing.T) {
		gotest.MatchSnapshot(t, "hello", "my-label")
	})

	snapPath := filepath.Join(dir, "TestMatchSnapshot_CustomName.snap")
	data, err := os.ReadFile(snapPath)
	gotest.NoError(t, err)
	gotest.True(t, strings.Contains(string(data), "=== SNAP case my-label ==="))
}

func TestMatchSnapshot_TopLevelOnly(t *testing.T) {
	dir := snapshotDir()
	t.Cleanup(func() { os.RemoveAll(dir) })

	gotest.MatchSnapshot(t, "top-level-value")

	snapPath := filepath.Join(dir, "TestMatchSnapshot_TopLevelOnly.snap")
	data, err := os.ReadFile(snapPath)
	gotest.NoError(t, err)
	gotest.True(t, strings.Contains(string(data), "=== SNAP _ ==="))
	gotest.True(t, strings.Contains(string(data), "top-level-value"))
}

func TestMatchSnapshot_SectionsAreSorted(t *testing.T) {
	dir := snapshotDir()
	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Run("zebra", func(t *testing.T) {
		gotest.MatchSnapshot(t, "z-val")
	})
	t.Run("alpha", func(t *testing.T) {
		gotest.MatchSnapshot(t, "a-val")
	})

	snapPath := filepath.Join(dir, "TestMatchSnapshot_SectionsAreSorted.snap")
	data, err := os.ReadFile(snapPath)
	gotest.NoError(t, err)

	content := string(data)
	alphaIdx := strings.Index(content, "=== SNAP alpha ===")
	zebraIdx := strings.Index(content, "=== SNAP zebra ===")
	gotest.True(t, alphaIdx < zebraIdx, "alpha section should come before zebra")
}

func TestMatchSnapshot_UpdateMode(t *testing.T) {
	dir := snapshotDir()
	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Run("entry", func(t *testing.T) {
		gotest.MatchSnapshot(t, "original")
	})

	t.Setenv("GOTEST_UPDATE_SNAPSHOTS", "1")
	t.Run("entry", func(t *testing.T) {
		gotest.MatchSnapshot(t, "updated")
	})

	snapPath := filepath.Join(dir, "TestMatchSnapshot_UpdateMode.snap")
	data, err := os.ReadFile(snapPath)
	gotest.NoError(t, err)

	content := string(data)
	gotest.True(t, strings.Contains(content, "updated"))
	gotest.False(t, strings.Contains(content, "original"))
}
