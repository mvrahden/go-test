//go:build linux

package gotestruntime

import (
	"bytes"
	"os"
	"strconv"
)

// othersInGroup reports whether a process of group pgid besides this one is
// anything but a zombie, reading /proc. When /proc cannot be read it reports
// true, so the group gets its whole grace.
func othersInGroup(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return true
	}
	self := os.Getpid()
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		if state, group, ok := parseStat(stat); ok && group == pgid && state != 'Z' && state != 'X' {
			return true
		}
	}
	return false
}

// parseStat reads the state and process group from /proc/<pid>/stat. The
// command name in parentheses may hold spaces and parentheses, so fields are
// counted from the last ')'.
func parseStat(stat []byte) (state byte, pgid int, ok bool) {
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 || i+2 >= len(stat) {
		return 0, 0, false
	}
	fields := bytes.Fields(stat[i+2:])
	// state ppid pgrp ...
	if len(fields) < 3 || len(fields[0]) != 1 {
		return 0, 0, false
	}
	g, err := strconv.Atoi(string(fields[2]))
	if err != nil {
		return 0, 0, false
	}
	return fields[0][0], g, true
}
