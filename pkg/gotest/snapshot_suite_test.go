package gotest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type snapshotTextMarshaler string

func (m snapshotTextMarshaler) MarshalText() ([]byte, error) { return []byte(m), nil }

type snapshotStringer string

func (s snapshotStringer) String() string { return string(s) }

type snapshotJSONMarshaler struct{ data any }

func (m snapshotJSONMarshaler) MarshalJSON() ([]byte, error) { return json.Marshal(m.data) }

type snapshotNamedString string

type SnapshotTestSuite struct{}

func (s *SnapshotTestSuite) TestMatchSnapshot(t *gotest.T) {
	snapDir := filepath.Join("testdata", "__snapshots__")
	t.T().Cleanup(func() { os.RemoveAll(snapDir) })

	t.When("no snapshot exists", func(w *gotest.T) {
		w.It("creates a grouped snapshot file on first run", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "hello world")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			gotest.True(it, strings.Contains(string(data), "hello world"))
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
			gotest.True(it, strings.Contains(string(data), "my-snapshot"))
		})
	})

	t.When("update mode is enabled", func(w *gotest.T) {
		w.It("overwrites the existing snapshot", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "original")

			it.T().Setenv("GOTEST_UPDATE_SNAPSHOTS", "1")
			gotest.MatchSnapshot(it, "updated")

			snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")
			data, err := os.ReadFile(snapPath)
			gotest.NoError(it, err)
			content := string(data)
			gotest.True(it, strings.Contains(content, "updated"))
		})
	})
}

func (s *SnapshotTestSuite) TestSnapshotContent(t *gotest.T) {
	snapDir := filepath.Join("testdata", "__snapshots__")

	t.T().Cleanup(func() { os.RemoveAll(snapDir) })
	snapPath := filepath.Join(snapDir, "TestSnapshotTestSuite_ext.snap")

	t.When("value is a string", func(w *gotest.T) {
		w.It("snapshots the string directly", func(it *gotest.T) {
			gotest.MatchSnapshot(it, "plain text")
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "plain text")
		})
	})

	t.When("value is []byte", func(w *gotest.T) {
		w.It("snapshots the byte content as text", func(it *gotest.T) {
			gotest.MatchSnapshot(it, []byte("raw bytes"))
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "raw bytes")
		})
	})

	t.When("value is a TextMarshaler", func(w *gotest.T) {
		w.It("uses MarshalText output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotTextMarshaler("marshaled text"))
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "marshaled text")
		})
	})

	t.When("value is a Stringer", func(w *gotest.T) {
		w.It("uses String output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotStringer("display text"))
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "display text")
		})
	})

	t.When("value is a *bytes.Buffer", func(w *gotest.T) {
		w.It("uses String without consuming the buffer", func(it *gotest.T) {
			buf := bytes.NewBufferString("buffer content")
			gotest.MatchSnapshot(it, buf)
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "buffer content")
			gotest.Equal(it, "buffer content", buf.String())
		})
	})

	t.When("value is a json.Marshaler", func(w *gotest.T) {
		w.It("pretty-prints the JSON output", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotJSONMarshaler{data: map[string]int{"a": 1}})
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "{\n  \"a\": 1\n}")
		})
	})

	t.When("value is json.RawMessage", func(w *gotest.T) {
		w.It("pretty-prints the raw JSON", func(it *gotest.T) {
			gotest.MatchSnapshot(it, json.RawMessage(`{"b":2,"a":1}`))
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "\"b\": 2")
			gotest.Contains(it, string(data), "\"a\": 1")
		})
	})

	t.When("value is an error", func(w *gotest.T) {
		w.It("snapshots the error message", func(it *gotest.T) {
			gotest.MatchSnapshot(it, fmt.Errorf("something went wrong"))
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "something went wrong")
		})
	})

	t.When("value is a seekable io.Reader", func(w *gotest.T) {
		w.It("reads content and restores the reader position", func(it *gotest.T) {
			r := strings.NewReader("reader content")
			gotest.MatchSnapshot(it, r)
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "reader content")
			again, _ := io.ReadAll(r)
			gotest.Equal(it, "reader content", string(again))
		})
	})

	t.When("value is a non-seekable io.Reader", func(w *gotest.T) {
		w.It("reads content (consuming the reader)", func(it *gotest.T) {
			r := io.NopCloser(strings.NewReader("pipe content"))
			gotest.MatchSnapshot(it, r)
			data, _ := os.ReadFile(snapPath)
			gotest.Contains(it, string(data), "pipe content")
		})
	})

	t.When("value is a named string type", func(w *gotest.T) {
		w.It("snapshots via reflect", func(it *gotest.T) {
			gotest.MatchSnapshot(it, snapshotNamedString("typed value"))
			data, _ := os.ReadFile(snapPath)
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
