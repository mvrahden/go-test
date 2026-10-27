package fixalinger

import (
	"bytes"
	"os"
	"strconv"
)

// alive reports whether a process with pid runs: an exited one its new
// parent has not reaped yet is a zombie, not running.
func alive(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	i := bytes.LastIndexByte(stat, ')')
	return i < 0 || i+2 >= len(stat) || stat[i+2] != 'Z'
}
