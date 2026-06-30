package gotest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotGroupingTestSuite tests grouped snapshot file creation with per-subtest sections.
type SnapshotGroupingTestSuite struct{ snapPath string }

func (s *SnapshotGroupingTestSuite) BeforeEach(_ *gotest.T) {
	s.snapPath = filepath.Join("testdata", "__snapshots__", "TestSnapshotGroupingTestSuite_ext.snap")
}

func (s *SnapshotGroupingTestSuite) AfterEach(_ *gotest.T) { os.Remove(s.snapPath) }

func (s *SnapshotGroupingTestSuite) TestMatchSnapshotGrouping(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	snapDir := filepath.Join("testdata", "__snapshots__")

	t.When("subtests write snapshots", func(w *gotest.T) {
		w.It("creates a file with named sections per subtest", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "value-one")
			snapPath := filepath.Join(snapDir, "TestSnapshotGroupingTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.Contains(it, content, "value-one")
		})

		w.It("creates a file with named sections per subtest (second)", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "value-two")
			snapPath := filepath.Join(snapDir, "TestSnapshotGroupingTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.Contains(it, content, "value-two")
		})
	})

	t.When("a top-level snapshot is written", func(w *gotest.T) {
		w.It("uses the _ section key for top-level callers", func(it *gotest.T) {
			snapPath := filepath.Join(snapDir, "TestSnapshotGroupingTestSuite_ext.snap")
			gotest.MatchSnapshot(it, "top-level-value")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "top-level-value")
		})
	})

	t.When("subtests write in non-alphabetical order", func(w *gotest.T) {
		w.It("zebra comes first alphabetically after alpha", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "z-val")
		})
	})

	t.When("subtests write in non-alphabetical order (alpha)", func(w *gotest.T) {
		w.It("alpha section appears before zebra in the file", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "a-val")
			snapPath := filepath.Join(snapDir, "TestSnapshotGroupingTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.Contains(it, content, "a-val")
			gotest.Contains(it, content, "z-val")
		})
	})
}

// SnapshotConcurrencyTestSuite tests concurrent snapshot writes from parallel subtests.
type SnapshotConcurrencyTestSuite struct{ snapPath string }

func (s *SnapshotConcurrencyTestSuite) BeforeEach(_ *gotest.T) {
	s.snapPath = filepath.Join("testdata", "__snapshots__", "TestSnapshotConcurrencyTestSuite_ext.snap")
}

func (s *SnapshotConcurrencyTestSuite) AfterEach(_ *gotest.T) { os.Remove(s.snapPath) }

func (s *SnapshotConcurrencyTestSuite) TestMatchSnapshotConcurrency(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")

	t.When("multiple goroutines write concurrently", func(w *gotest.T) {
		for i := range 10 {
			w.It(fmt.Sprintf("goroutine %d writes its value", i), func(it *gotest.T) {
				it.T().Parallel() //nolint:t-escape
				gotest.MatchSnapshot(it, fmt.Sprintf("concurrent-value-%d", i))
			})
		}
	})
}

// SnapshotUpdateTestSuite tests snapshot update mode via GOTEST_UPDATE_SNAPSHOTS.
type SnapshotUpdateTestSuite struct{ snapPath string }

func (s *SnapshotUpdateTestSuite) BeforeEach(_ *gotest.T) {
	s.snapPath = filepath.Join("testdata", "__snapshots__", "TestSnapshotUpdateTestSuite_ext.snap")
}

func (s *SnapshotUpdateTestSuite) AfterEach(_ *gotest.T) { os.Remove(s.snapPath) }

func (s *SnapshotUpdateTestSuite) TestMatchSnapshotUpdate(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	snapDir := filepath.Join("testdata", "__snapshots__")

	t.When("GOTEST_UPDATE_SNAPSHOTS is set", func(w *gotest.T) {
		w.It("replaces original content with updated content and removes the old value", func(it *gotest.T) {
			snapPath := filepath.Join(snapDir, "TestSnapshotUpdateTestSuite_ext.snap")
			gotest.MatchSnapshot(it, "original-value")

			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "original-value")

			it.Setenv(protocol.EnvUpdateSnapshots, "1")
			gotest.MatchSnapshot(it, "updated-value")

			data, err = os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.Contains(it, content, "updated-value")
			gotest.NotContains(it, content, "original-value", "original content should be replaced")
		})
	})
}

type snapshotTextMarshaler string

func (m snapshotTextMarshaler) MarshalText() ([]byte, error) { return []byte(m), nil }

type snapshotStringer string

func (s snapshotStringer) String() string { return string(s) }

type snapshotJSONMarshaler struct{ data any }

func (m snapshotJSONMarshaler) MarshalJSON() ([]byte, error) { return json.Marshal(m.data) }

type snapshotNamedString string

// SnapshotTestSuite tests snapshot matching, custom naming, and value serialization.
type SnapshotTestSuite struct{ snapPath string }

func (s *SnapshotTestSuite) BeforeEach(_ *gotest.T) {
	s.snapPath = filepath.Join("testdata", "__snapshots__", "TestSnapshotTestSuite_ext.snap")
}

func (s *SnapshotTestSuite) AfterEach(_ *gotest.T) { os.Remove(s.snapPath) }

func (s *SnapshotTestSuite) TestMatchSnapshot(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	snapDir := filepath.Join("testdata", "__snapshots__")

	t.When("no snapshot exists", func(w *gotest.T) {
		w.It("creates a grouped snapshot file on first run", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "hello world")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "hello world")
		})
	})

	t.When("snapshot already exists", func(w *gotest.T) {
		w.It("matches multiple snapshots with dedup suffixes", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "stable value")
			gotest.MatchSnapshot(it, "stable value")
		})
	})

	t.When("custom name is provided", func(w *gotest.T) {
		w.It("uses the custom name in the section key", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "custom content", "my-snapshot")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.Contains(it, string(data), "my-snapshot")
		})
	})

	t.When("update mode is enabled", func(w *gotest.T) {
		w.It("overwrites the existing snapshot", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "original")

			it.Setenv(protocol.EnvUpdateSnapshots, "1")
			gotest.MatchSnapshot(it, "updated")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.Contains(it, content, "updated")
		})
	})
}

func (s *SnapshotTestSuite) TestSnapshotContent(t *gotest.T) {
	t.Setenv(protocol.EnvCI, "0")
	snapDir := filepath.Join("testdata", "__snapshots__")
	snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")

	t.When("value is a string", func(w *gotest.T) {
		w.It("snapshots the string directly", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "plain text")
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "plain text")
		})
	})

	t.When("value is []byte", func(w *gotest.T) {
		w.It("snapshots the byte content as text", func(it *gotest.T) {
			gotest.MatchSnapshot(it, []byte("raw bytes"))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "raw bytes")
		})
	})

	t.When("value is a TextMarshaler", func(w *gotest.T) {
		w.It("uses MarshalText output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotTextMarshaler("marshaled text"))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "marshaled text")
		})
	})

	t.When("value is a Stringer", func(w *gotest.T) {
		w.It("uses String output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotStringer("display text"))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "display text")
		})
	})

	t.When("value is a *bytes.Buffer", func(w *gotest.T) {
		w.It("uses String without consuming the buffer", func(it *gotest.T) {
			buf := bytes.NewBufferString("buffer content")
			gotest.MatchSnapshot(it, buf)
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "buffer content")
			gotest.Equal(it, "buffer content", buf.String())
		})
	})

	t.When("value is a json.Marshaler", func(w *gotest.T) {
		w.It("pretty-prints the JSON output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotJSONMarshaler{data: map[string]int{"a": 1}})
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "{\n  \"a\": 1\n}")
		})
	})

	t.When("value is json.RawMessage", func(w *gotest.T) {
		w.It("pretty-prints the raw JSON", func(it *gotest.T) {
			gotest.MatchSnapshot(it, json.RawMessage(`{"b":2,"a":1}`))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "\"b\": 2")
			gotest.Contains(it, string(data), "\"a\": 1")
		})
	})

	t.When("value is an error", func(w *gotest.T) {
		w.It("snapshots the error message", func(it *gotest.T) {
			gotest.MatchSnapshot(it, fmt.Errorf("something went wrong"))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "something went wrong")
		})
	})

	t.When("value is a seekable io.Reader", func(w *gotest.T) {
		w.It("reads content and restores the reader position", func(it *gotest.T) {
			r := strings.NewReader("reader content")
			gotest.MatchSnapshot(it, r)
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "reader content")
			again := gotest.Must(io.ReadAll(r))
			gotest.Equal(it, "reader content", string(again))
		})
	})

	t.When("value is a non-seekable io.Reader", func(w *gotest.T) {
		w.It("reads content (consuming the reader)", func(it *gotest.T) {
			r := io.NopCloser(strings.NewReader("pipe content"))
			gotest.MatchSnapshot(it, r)
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "pipe content")
		})
	})

	t.When("value is a named string type", func(w *gotest.T) {
		w.It("snapshots via reflect", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotNamedString("typed value"))
			data := gotest.Must(os.ReadFile(snapPath))
			gotest.Contains(it, string(data), "typed value")
		})
	})

	t.When("value is nil", func(w *gotest.T) {
		w.It("fails with an error", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, nil) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "unsupported snapshot value")
		})
	})

	t.When("value is a nil pointer", func(w *gotest.T) {
		w.It("fails with an error", func(it *gotest.T) {
			var buf *bytes.Buffer
			m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, buf) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "unsupported snapshot value")
		})
	})

	t.When("value is an unsupported type", func(w *gotest.T) {
		w.It("fails with a descriptive error", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, 42) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "unsupported snapshot type")
		})
	})
}
