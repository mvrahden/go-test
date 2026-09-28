package gotestrunner

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// test2jsonPath names the converter binary, resolved once per run: the go
// front-end costs 60-145ms per launch, the tool itself under 5ms. It is
// empty when the path cannot be resolved.
var test2jsonPath = sync.OnceValue(func() string {
	return resolveTest2JSON(func() ([]byte, error) {
		return exec.Command("go", "tool", "-n", "test2json").Output()
	})
})

func resolveTest2JSON(locate func() ([]byte, error)) string {
	out, err := locate()
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	return path
}

// test2jsonArgv wraps a test binary invocation with the converter; without
// a resolved path it goes through the go tool launcher.
func test2jsonArgv(converter string, target SuiteTarget, testArgs []string) []string { //nolint:gocritic // hugeParam: stable API
	argv := []string{converter}
	if converter == "" {
		argv = []string{"go", "tool", "test2json"}
	}
	argv = append(argv, "-p", target.Package, "-t", target.BinaryPath)
	return append(argv, testArgs...)
}
