package gotestruntime

import "encoding/json"

// ExportLoadSharedState reads the state file the runner wrote for a suite.
func ExportLoadSharedState(path string) (map[string]json.RawMessage, error) {
	return loadSharedState(path)
}
