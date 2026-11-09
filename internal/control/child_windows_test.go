//go:build windows

package control_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/pkg/gotest"
)

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procGetHandleInformation         = kernel32.NewProc("GetHandleInformation")
	procGetFileInformationByHandleEx = kernel32.NewProc("GetFileInformationByHandleEx")
)

var spawnWays = []way{
	{"exec", func(env []string) error {
		cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = env
		return cmd.Start()
	}},
	{"shell", func(env []string) error {
		cmd := exec.Command("cmd", "/c", os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = env
		return cmd.Start()
	}},
	// A process started the way most non-Go programs start one: every
	// inheritable handle goes along, not only a listed few.
	{"inherit", inheritAll},
}

func inheritAll(env []string) error {
	exe, err := syscall.UTF16PtrFromString(os.Args[0])
	if err != nil {
		return err
	}
	cmdline, err := syscall.UTF16PtrFromString(syscall.EscapeArg(os.Args[0]))
	if err != nil {
		return err
	}
	block := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	si := &syscall.StartupInfo{Flags: syscall.STARTF_USESTDHANDLES}
	si.Cb = uint32(unsafe.Sizeof(*si))
	var pi syscall.ProcessInformation
	const createUnicodeEnvironment = 0x400
	if err := syscall.CreateProcess(exe, cmdline, nil, nil, true, createUnicodeEnvironment, &block[0], nil, si, &pi); err != nil {
		return err
	}
	_ = syscall.CloseHandle(pi.Thread)
	_ = syscall.CloseHandle(pi.Process)
	return nil
}

// identity is what tells the channel's pipe apart at handle h: its name.
func identity(h uintptr) (string, bool) {
	if t, err := syscall.GetFileType(syscall.Handle(h)); err != nil || t != syscall.FILE_TYPE_PIPE {
		return "", false
	}
	// FILE_NAME_INFO: the name's length in bytes, then the name in UTF-16.
	var buf [260]uint32
	const fileNameInfo = 2
	r, _, _ := procGetFileInformationByHandleEx.Call(h, fileNameInfo, uintptr(unsafe.Pointer(&buf[0])), unsafe.Sizeof(buf))
	if r == 0 {
		return "", false
	}
	n := min(int(buf[0])/2, (len(buf)-1)*2)
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[1])), n)), true
}

func channelHandles(raw string) []uintptr {
	var hs []uintptr
	for s := range strings.SplitSeq(raw, ",") {
		if h, err := strconv.ParseUint(s, 10, 64); err == nil {
			hs = append(hs, uintptr(h))
		}
	}
	return hs
}

func identify(raw string) []string {
	var ids []string
	for _, h := range channelHandles(raw) {
		id, _ := identity(h)
		ids = append(ids, id)
	}
	return ids
}

// reportClaimed reports whether the handle may still be inherited and
// returns what identifies it.
func reportClaimed(e *control.Endpoint, raw string, ids []string) string {
	r, w := e.Handles()
	report("inherit", inheritable(r) || inheritable(w))
	hs := channelHandles(raw)
	if len(ids) != 2 || len(hs) != 2 {
		return ""
	}
	return fmt.Sprintf("%d=%s,%d=%s", hs[0], ids[0], hs[1], ids[1])
}

func inheritable(h uintptr) bool {
	var flags uint32
	r, _, _ := procGetHandleInformation.Call(h, uintptr(unsafe.Pointer(&flags)))
	return r != 0 && flags&syscall.HANDLE_FLAG_INHERIT != 0
}

// held lists the handles of expect that hold the channel's pipe here.
func held(expect string) string {
	var hs []string
	for pair := range strings.SplitSeq(expect, ",") {
		s, id, _ := strings.Cut(pair, "=")
		h, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			continue
		}
		if got, ok := identity(uintptr(h)); ok && got == id {
			hs = append(hs, s)
		}
	}
	if len(hs) == 0 {
		return "none"
	}
	return strings.Join(hs, ",")
}

// untouched reports whether the handle raw names is still a regular file.
func untouched(raw string) bool {
	hs := channelHandles(raw)
	for _, h := range hs {
		if t, err := syscall.GetFileType(syscall.Handle(h)); err != nil || t != syscall.FILE_TYPE_DISK {
			return false
		}
	}
	return len(hs) > 0
}

// standardValue names stdout, a pipe from the runner.
func standardValue() string {
	out, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	return strconv.FormatUint(uint64(out), 10)
}

// attachFile lets the child inherit f and returns the value naming it.
func attachFile(cmd *exec.Cmd, f *os.File) string {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	h := syscall.Handle(f.Fd())
	_ = syscall.SetHandleInformation(h, syscall.HANDLE_FLAG_INHERIT, syscall.HANDLE_FLAG_INHERIT)
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, h)
	return strconv.FormatUint(uint64(h), 10)
}

// socketStdout leaves stdout a pipe: on Windows a standard stream is a pipe
// already, which passes the channel's type check.
func socketStdout(*exec.Cmd) (r, w *os.File, err error) { return nil, nil, nil }

// prepareAttach gives cmd process attributes and a handle of its own.
func prepareAttach(cmd *exec.Cmd) {
	h, _ := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:              syscall.CREATE_NEW_PROCESS_GROUP,
		AdditionalInheritedHandles: []syscall.Handle{h},
	}
}

// checkAttached checks that the channel's two handles follow cmd's own and that
// the attributes cmd had are kept.
func checkAttached(t *gotest.T, cmd *exec.Cmd, entry string, prepared bool) {
	gotest.NotZero(t, cmd.SysProcAttr)
	hs := cmd.SysProcAttr.AdditionalInheritedHandles
	own := 0
	if prepared {
		own = 1
		gotest.Equal(t, uint32(syscall.CREATE_NEW_PROCESS_GROUP), cmd.SysProcAttr.CreationFlags)
	}
	gotest.Len(t, hs, own+2)
	gotest.Equal(t, fmt.Sprintf("%s=%d,%d", control.Env, hs[own], hs[own+1]), entry)
}

// checkClaimed checks that the handle may not be inherited any more.
func checkClaimed(t *gotest.T, c *child) {
	gotest.Equal(t, "false", c.seen["inherit"])
}

func lowerLimit(string) error { return errors.New("no descriptor limit on Windows") }
