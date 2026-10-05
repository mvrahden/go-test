//go:build linux

package proctree

import (
	"bytes"
	"os"
	"strconv"
)

// groupHasLiveMember reports whether a process in group pgid is anything but a
// zombie, reading /proc; known is false when /proc cannot be read.
func groupHasLiveMember(pgid int) (live, known bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, false
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		state, group, ok := parseStat(stat)
		if ok && group == pgid && state != 'Z' && state != 'X' {
			return true, true
		}
	}
	return false, true
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
