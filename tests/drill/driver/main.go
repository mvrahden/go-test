// Command driver is the mutation drill's orchestrator; tests/drill/drill.sh
// builds it from the unmodified tree and runs it.
package main

import (
	"os"
	"runtime"

	"github.com/mvrahden/go-test/internal/drill"
)

// The signal watcher runs on the main thread.
func init() { runtime.LockOSThread() }

func main() { os.Exit(drill.Main(os.Args[1:])) }
