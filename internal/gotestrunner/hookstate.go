package gotestrunner

import "path/filepath"

// HookStateFile is the file gotest hook keeps what it told an agent about
// the checkout at top in, under the gotest cache.
func HookStateFile(top string) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hook", auditBaseKey(top)+".json"), nil
}
