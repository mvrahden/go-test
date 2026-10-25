package proctree

import (
	"fmt"
	"strconv"
)

// maxExitCode is the highest status a program sets as its own exit code.
// Windows allows any 32-bit value, but a termination status uses the high bits
// (0xC000013A), so anything above the conventional range is a termination.
const maxExitCode = 255

// statusNames names the Windows termination statuses a run meets.
var statusNames = map[uint32]string{
	0xC0000005: "STATUS_ACCESS_VIOLATION",
	0xC00000FD: "STATUS_STACK_OVERFLOW",
	0xC000013A: "STATUS_CONTROL_C_EXIT",
	0xC0000409: "STATUS_STACK_BUFFER_OVERRUN",
}

// Terminated reports whether status is one a process cannot have chosen as
// its exit code: a Windows termination status.
func Terminated(status int) bool { return status > maxExitCode }

// StatusHex spells a termination status as Windows does, 0xC000013A.
func StatusHex(status int) string {
	return fmt.Sprintf("0x%X", uint32(status)) //nolint:gosec // G115: a Windows exit status is a DWORD
}

// ExitStatus spells status for a message: in decimal when the process chose
// it, in hex with its name, when it has one, when it is a termination status.
func ExitStatus(status int) string {
	if !Terminated(status) {
		return strconv.Itoa(status)
	}
	if name, ok := statusNames[uint32(status)]; ok { //nolint:gosec // G115: a Windows exit status is a DWORD
		return StatusHex(status) + " (" + name + ")"
	}
	return StatusHex(status)
}
