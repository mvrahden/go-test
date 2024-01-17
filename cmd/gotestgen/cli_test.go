package gotestgen

import (
	"testing"
)

// Injects a noop func to deactivate file deletion
func PatchDeleteOldGeneratedFileFunc(t *testing.T) {
	var fn = findAndDeleteOldGeneratedFile
	t.Cleanup(func() {
		findAndDeleteOldGeneratedFile = fn
	})
	findAndDeleteOldGeneratedFile = func(dir string) error {
		return nil
	}
}

// Injects an alternative func to intercept target filename determination
// and replace directory with test directory
func PatchTargetFilenameFunc(t *testing.T, targetDirectory string) {
	var fn = targetFilename
	t.Cleanup(func() {
		targetFilename = fn
	})
	targetFilename = func(_, file string) string {
		return fn(targetDirectory, file)
	}
}
