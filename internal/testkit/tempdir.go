package testkit

import "github.com/mvrahden/go-test/pkg/gotest"

// IsolateTempDir points the temp dir at one of t's own, so no sweep a test
// runs reaches the work and overlay dirs of other runs.
func IsolateTempDir(t *gotest.T) {
	dir := t.TempDir()
	for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(env, dir)
	}
}
