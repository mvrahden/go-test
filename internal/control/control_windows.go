//go:build windows

package control

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	pipeAccessDuplex        = 0x3
	fileFlagFirstPipeInst   = 0x80000
	pipeRejectRemoteClients = 0x8
	pipeBufferSize          = 4096
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procCreateNamedPipe = kernel32.NewProc("CreateNamedPipeW")
	procPeekNamedPipe   = kernel32.NewProc("PeekNamedPipe")
)

// ChildEnd is what the suite process inherits: the client end of each pipe,
// down for the lines it reads, up for the lines it writes.
type ChildEnd struct {
	down, up syscall.Handle
	once     sync.Once
}

// New creates the channel for one suite process: two overlapped named pipes,
// each with a single client opened at once and each used one way. One pipe
// would do for the bytes, but a file read on one goroutine and written on
// another races on the offset Go's descriptor keeps for it, and a file object
// binds to one completion port only, so a duplicate handle cannot be bound
// apart. Each pipe is still opened both ways: Go gives a handle deadlines
// only once it has read its completion modes, which takes read access, so a
// write-only end could not be written with a deadline. The client handles
// are inheritable; the servers are not.
func New() (*Link, *ChildEnd, error) {
	down, err := pipe()
	if err != nil {
		return nil, nil, err
	}
	up, err := pipe()
	if err != nil {
		_ = syscall.CloseHandle(down.server)
		_ = syscall.CloseHandle(down.client)
		return nil, nil, err
	}
	r := os.NewFile(uintptr(up.server), "gotest-control")
	w := os.NewFile(uintptr(down.server), "gotest-control")
	return newLink(r, w), &ChildEnd{down: down.client, up: up.client}, nil
}

// pipeEnds are the two handles of one pipe: the server stays with the
// runner, the client is inherited.
type pipeEnds struct{ server, client syscall.Handle }

// pipe makes one overlapped duplex named pipe of a random local name and its
// only client, inheritable.
func pipe() (pipeEnds, error) {
	name, err := pipeName()
	if err != nil {
		return pipeEnds{}, err
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return pipeEnds{}, err
	}
	r, _, e := procCreateNamedPipe.Call(uintptr(unsafe.Pointer(p)),
		pipeAccessDuplex|syscall.FILE_FLAG_OVERLAPPED|fileFlagFirstPipeInst,
		pipeRejectRemoteClients, 1, pipeBufferSize, pipeBufferSize, 0, 0)
	server := syscall.Handle(r)
	if server == syscall.InvalidHandle {
		return pipeEnds{}, os.NewSyscallError("CreateNamedPipe", e)
	}
	sa := &syscall.SecurityAttributes{InheritHandle: 1}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	client, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, sa, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		_ = syscall.CloseHandle(server)
		return pipeEnds{}, os.NewSyscallError("CreateFile", err)
	}
	return pipeEnds{server: server, client: client}, nil
}

func pipeName() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf(`\\.\pipe\gotest-control-%d-%s`, os.Getpid(), hex.EncodeToString(b[:])), nil
}

// Attach lists both handles among those cmd inherits, keeping any SysProcAttr
// already set, and names them in cmd's environment, down first.
func (c *ChildEnd) Attach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, c.down, c.up)
	setEnv(cmd, value(c.down, c.up))
}

// value spells the two handles as the variable carries them.
func value(down, up syscall.Handle) string {
	return strconv.FormatUint(uint64(down), 10) + "," + strconv.FormatUint(uint64(up), 10)
}

// Close closes both ends in the runner, once the command has started.
func (c *ChildEnd) Close() {
	c.once.Do(func() {
		_ = syscall.CloseHandle(c.down)
		_ = syscall.CloseHandle(c.up)
	})
}

// claim takes the two pipe handles v names, down then up. The process's
// standard handles and anything but a pipe are left alone.
func claim(v string) *Endpoint {
	downV, upV, ok := strings.Cut(v, ",")
	if !ok {
		return nil
	}
	down, okDown := handle(downV)
	up, okUp := handle(upV)
	if !okDown || !okUp || down == up {
		return nil
	}
	for _, h := range [...]syscall.Handle{down, up} {
		if syscall.SetHandleInformation(h, syscall.HANDLE_FLAG_INHERIT, 0) != nil {
			return nil
		}
	}
	return newEndpoint(os.NewFile(uintptr(down), "gotest-control"), os.NewFile(uintptr(up), "gotest-control"))
}

// handle reads one handle value: a pipe that is none of the standard ones.
func handle(v string) (syscall.Handle, bool) {
	n, ok := decimal(v)
	h := syscall.Handle(n)
	if !ok || n == 0 || standard(h) {
		return 0, false
	}
	if t, err := syscall.GetFileType(h); err != nil || t != syscall.FILE_TYPE_PIPE {
		return 0, false
	}
	return h, true
}

func standard(h syscall.Handle) bool {
	for _, std := range [...]int{syscall.STD_INPUT_HANDLE, syscall.STD_OUTPUT_HANDLE, syscall.STD_ERROR_HANDLE} {
		if s, err := syscall.GetStdHandle(std); err == nil && s == h {
			return true
		}
	}
	return false
}

// readNow reads what the pipe holds without waiting: a read with a deadline
// already past would not read at all.
func readNow(f *os.File, p []byte) (n int, eof bool) {
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, true
	}
	var avail uint32
	var perr error
	if err := rc.Control(func(h uintptr) {
		r, _, e := procPeekNamedPipe.Call(h, 0, 0, 0, uintptr(unsafe.Pointer(&avail)), 0)
		if r == 0 {
			perr = e
		}
	}); err != nil || perr != nil {
		return 0, true
	}
	if avail == 0 {
		return 0, false
	}
	n, err = f.Read(p[:min(int(avail), len(p))])
	if n == 0 && err != nil {
		return 0, true
	}
	return n, false
}
