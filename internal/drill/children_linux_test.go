package drill_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/drill"
)

// runChild is a child program:
//   - drill <args...>: the drill's own command line
//   - detach <pidfile>: starts a process in a session of its own that starts
//     a sleeper and exits, so the sleeper is orphaned; waits for its pid
//   - session <pidfile>: starts a sleeper in a session of its own, publishes
//     its pid and sleeps
//   - loop <logs> <phases>: judges synthetic workers under real supervisors
//   - worker <name> <result> <leftover>: a synthetic worker, by name
func runChild(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "drill":
		return drill.Main(args[1:])
	case "detach":
		cmd := childCommand("orphan", args[1])
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if cmd.Run() != nil {
			return 2
		}
		for !exists(args[1]) {
			time.Sleep(10 * time.Millisecond)
		}
		return 0
	case "orphan":
		if childCommand("sleeper", args[1]).Start() != nil {
			return 2
		}
		return 0
	case "sleeper":
		if publish(args[1], os.Getpid()) != nil {
			return 2
		}
		time.Sleep(time.Minute)
		return 0
	case "session":
		cmd := childCommand("sleep")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if cmd.Start() != nil || publish(args[1], cmd.Process.Pid) != nil {
			return 2
		}
		time.Sleep(time.Minute)
		return 0
	case "sleep":
		time.Sleep(time.Minute)
		return 0
	case "loop":
		return loop(args[1], args[2])
	case "worker":
		return syntheticWorker(args[1], args[2], args[3])
	}
	return 2
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// publish writes pid to path by rename, so path is never seen empty.
func publish(path string, pid int) error {
	if err := os.WriteFile(path+".tmp", []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// syntheticWorker takes down its supervisor or its own group, or signals
// the drill, by name; the others succeed.
func syntheticWorker(name, result, leftover string) int {
	switch name {
	case "b":
		_ = publish(leftover, os.Getpid())
		_ = syscall.Kill(os.Getppid(), syscall.SIGKILL)
		time.Sleep(time.Minute)
	case "c":
		_ = syscall.Kill(0, syscall.SIGKILL)
	case "d":
		pid, _ := strconv.Atoi(os.Getenv("DRILL_PID"))
		_ = syscall.Kill(pid, syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
	case "g":
		_ = publish(leftover, os.Getpid())
		time.Sleep(time.Minute)
	}
	data, _ := json.Marshal(map[string]any{"name": name, "caught": true, "reason": "caught"})
	if os.WriteFile(result, data, 0o600) != nil {
		return 2
	}
	return 0
}

// loop is the drill's judging loop with real supervisors and synthetic
// workers.
func loop(logs, phasesJSON string) int {
	if err := drill.EnableSubreaper(); err != nil {
		fmt.Println(err)
		return 2
	}
	var phases []drill.Phase
	if err := json.Unmarshal([]byte(phasesJSON), &phases); err != nil {
		fmt.Println(err)
		return 2
	}
	_ = os.Setenv("DRILL_PID", strconv.Itoa(os.Getpid()))
	supervision := drill.NewSupervision([]string{self(), "drill"}, 20, logs)
	leftover := filepath.Join(logs, "leftover")
	execute := func(_ int, name string, _ int) (drill.Verdict, *drill.Status, error) {
		result := filepath.Join(logs, name+".result")
		out, err := os.Create(filepath.Join(logs, name+".out"))
		if err != nil {
			return drill.Verdict{}, nil, err
		}
		defer func() { _ = out.Close() }()
		code, observed, err := supervision.Run([]string{self(), "worker", name, result, leftover}, name, out, out, "")
		if err != nil {
			return drill.Verdict{}, nil, err
		}
		return drill.ReadResult(result, name, observed, code), observed, nil
	}
	results := map[string]drill.Verdict{}
	var err error
	drill.WatchSignals(supervision, func() { err = drill.JudgeAll(phases, supervision, execute, results) })
	if err == nil {
		err = drill.Clean(nil)
	}
	if err != nil {
		fmt.Println(err)
		return 1
	}
	report := loopReport{Stopped: supervision.Stopping(), Caught: map[string]bool{}}
	for name, r := range results {
		report.Caught[name] = r.Caught
	}
	data, _ := json.Marshal(report)
	fmt.Println(string(data))
	return 0
}
