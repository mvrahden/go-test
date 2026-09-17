//go:build windows

package proctree_test

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var interruptSignals = []os.Signal{os.Interrupt}

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

func printConsoleProcesses() int {
	pids := make([]uint32, 64)
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n == 0 || int(n) > len(pids) {
		return 2
	}
	for _, pid := range pids[:n] {
		fmt.Printf("pid=%d\n", pid)
	}
	fmt.Println("ready")
	return 0
}

// watchExit holds a handle to pid, so the pid cannot be reused while it is
// watched, and reports whether the process exited.
func watchExit(pid int) (exited func() bool, stop func(), err error) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // G115: a Windows process ID is a DWORD
	if err != nil {
		return nil, nil, err
	}
	exited = func() bool {
		event, err := windows.WaitForSingleObject(h, 0)
		return err == nil && event == windows.WAIT_OBJECT_0
	}
	return exited, func() { _ = windows.CloseHandle(h) }, nil
}
