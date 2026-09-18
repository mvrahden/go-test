//go:build windows

package proctree

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sysTree holds a handle to the root, which keeps its pid from being reused
// while the tree is owned, and the job every process of the tree runs in.
type sysTree struct {
	process windows.Handle
	job     windows.Handle
}

// prepare gives the command a hidden console of its own. A console control
// event reaches processes by console: on a shared one it can reach every
// process attached, the sender included (microsoft/terminal#335).
func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	cmd.Stdout = viaPipe(cmd.Stdout)
	cmd.Stderr = viaPipe(cmd.Stderr)
}

// viaPipe hides a console handle behind a plain writer, so exec copies the
// output through a pipe: a child on its own console never writes to a console
// handle of the caller's.
func viaPipe(w io.Writer) io.Writer {
	f, ok := w.(*os.File)
	if !ok {
		return w
	}
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) != nil {
		return w
	}
	return struct{ io.Writer }{f}
}

// adopt opens the root and puts it in a job that kills what is left of the
// tree when the job closes. A process the root starts afterwards joins the job
// on creation; the root runs no code of its own between its creation and here.
func (s *sysTree) adopt(pid int) {
	const access = windows.SYNCHRONIZE | windows.PROCESS_TERMINATE | windows.PROCESS_SET_QUOTA | windows.PROCESS_QUERY_LIMITED_INFORMATION
	process, err := windows.OpenProcess(access, false, uint32(pid)) //nolint:gosec // G115: a Windows process ID is a DWORD
	if err != nil {
		return
	}
	s.process = process

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	s.job = job
}

var errNoHandle = errors.New("proctree: the process could not be opened")

func (s *sysTree) interrupt(pid int) error {
	if s.process == 0 {
		return errNoHandle
	}
	if exited(s.process) {
		return os.ErrProcessDone
	}
	return interruptConsole(pid)
}

func (s *sysTree) kill(int) error {
	if s.process == 0 {
		return errNoHandle
	}
	if s.job != 0 {
		return windows.TerminateJobObject(s.job, 1)
	}
	if exited(s.process) {
		return os.ErrProcessDone
	}
	return windows.TerminateProcess(s.process, 1)
}

func (s *sysTree) release() {
	if s.job != 0 {
		_ = windows.CloseHandle(s.job)
	}
	if s.process != 0 {
		_ = windows.CloseHandle(s.process)
	}
}

func exited(process windows.Handle) bool {
	event, err := windows.WaitForSingleObject(process, 0)
	return err == nil && event == windows.WAIT_OBJECT_0
}
