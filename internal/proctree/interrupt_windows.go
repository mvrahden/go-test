//go:build windows

package proctree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// envInterruptConsole makes a run of an executable that links this package a
// helper: it interrupts the console of the process the variable names, then
// exits. See interruptConsole.
const envInterruptConsole = "GOTEST_INTERNAL_INTERRUPT_CONSOLE"

// Exit codes of a helper run.
const (
	helperSent      = 0
	helperFailed    = 1
	helperNoConsole = 3
)

// helperTimeout bounds a helper run; a first start of a large executable can
// wait on an antivirus scan.
const helperTimeout = time.Minute

func init() {
	if v, ok := syscall.Getenv(envInterruptConsole); ok {
		os.Exit(runHelper(v))
	}
}

// interruptConsole sends CTRL_BREAK to every process on the console of pid. A
// process can only send one to its own console, so a helper run of this
// executable, started without a console, attaches to pid's console and sends
// it there: the caller's console never carries the event.
func interruptConsole(pid int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe) //nolint:gosec // G204: this executable, run as its own helper
	cmd.Env = append(os.Environ(), envInterruptConsole+"="+strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return nil
	case !errors.As(err, &exitErr):
		return fmt.Errorf("interrupt the console of process %d: %w", pid, err)
	case exitErr.ExitCode() == helperNoConsole:
		return os.ErrProcessDone
	case uint32(exitErr.ExitCode()) == uint32(windows.STATUS_CONTROL_C_EXIT): //nolint:gosec // G115: an exit status is a DWORD
		// The helper's own copy of the event ended it, after it was sent.
		return nil
	}
	return fmt.Errorf("interrupt the console of process %d: helper exited %d", pid, exitErr.ExitCode())
}

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole         = kernel32.NewProc("AttachConsole")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// runHelper is the helper side of interruptConsole.
func runHelper(v string) int {
	pid, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return helperFailed
	}
	_, _, _ = procFreeConsole.Call()
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		// No such process, or it has no console any more: it exited.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_INVALID_HANDLE) || errors.Is(err, windows.ERROR_GEN_FAILURE) {
			return helperNoConsole
		}
		return helperFailed
	}
	// The event reaches the helper too; handle it so the default handler does
	// not end the helper before it reports.
	if r, _, _ := procSetConsoleCtrlHandler.Call(windows.NewCallback(func(uint32) uintptr { return 1 }), 1); r == 0 {
		return helperFailed
	}
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, 0); err != nil {
		return helperFailed
	}
	return helperSent
}
