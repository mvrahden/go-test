package proctree_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"testing"
	"time"
)

// envChild makes a run of this test binary one of the child programs below.
const envChild = "GOTEST_PROCTREE_CHILD"

const (
	envMarker      = "GOTEST_PROCTREE_MARKER"
	envGrandMarker = "GOTEST_PROCTREE_GRANDCHILD_MARKER"
	envSpawn       = "GOTEST_PROCTREE_SPAWN"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(envChild); mode != "" {
		os.Exit(runChild(mode))
	}
	os.Exit(m.Run())
}

// runChild is a child program. Each prints "ready" once it can be stopped.
//   - listen: writes its marker on a shutdown request and exits; with a
//     grandchild marker it first starts a listening grandchild and waits for it
//   - ignore: swallows shutdown requests; with spawn it first starts an
//     ignoring grandchild and prints its pid
//   - orphan: starts an ignoring grandchild, prints its pid and exits
//   - exit: exits as soon as it is ready
//   - consoles: prints the pids attached to its console (Windows)
func runChild(mode string) int {
	switch mode {
	case "listen":
		return listen()
	case "ignore":
		signal.Notify(make(chan os.Signal, 1), interruptSignals...)
		if os.Getenv(envSpawn) == "1" {
			if _, err := startGrandchild("ignore"); err != nil {
				return 2
			}
		}
		fmt.Println("ready")
		time.Sleep(5 * time.Minute)
		return 0
	case "orphan":
		if _, err := startGrandchild("ignore"); err != nil {
			return 2
		}
		fmt.Println("ready")
		return 0
	case "exit":
		fmt.Println("ready")
		return 0
	case "consoles":
		return printConsoleProcesses()
	}
	return 2
}

func listen() int {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, interruptSignals...)
	var grandchild *exec.Cmd
	if marker := os.Getenv(envGrandMarker); marker != "" {
		cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = append(childEnv("listen"), envMarker+"="+marker)
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return 2
		}
		if !waitReady(bufio.NewScanner(out)) {
			return 2
		}
		grandchild = cmd
	}
	fmt.Println("ready")
	<-sig
	if err := os.WriteFile(os.Getenv(envMarker), []byte("interrupted"), 0o600); err != nil {
		return 2
	}
	if grandchild != nil {
		_ = grandchild.Wait()
	}
	return 0
}

// startGrandchild starts a child program the way a test starts a helper: in
// its parent's tree, with no output.
func startGrandchild(mode string) (int, error) {
	cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
	cmd.Env = childEnv(mode)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	fmt.Printf("pid=%d\n", cmd.Process.Pid)
	return cmd.Process.Pid, nil
}

// childEnv is this process's environment running mode, without the variables
// that configured this process.
func childEnv(mode string) []string {
	env := []string{envChild + "=" + mode}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GOTEST_PROCTREE_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// waitReady reads lines until "ready".
func waitReady(sc *bufio.Scanner) bool {
	for sc.Scan() {
		if sc.Text() == "ready" {
			return true
		}
	}
	return false
}

// readUntilReady reads lines until "ready" and returns the pid lines' values.
func readUntilReady(sc *bufio.Scanner) (pids []int, ready bool) {
	for sc.Scan() {
		line := sc.Text()
		if line == "ready" {
			return pids, true
		}
		if v, ok := strings.CutPrefix(line, "pid="); ok {
			if pid, err := strconv.Atoi(v); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	return pids, false
}
