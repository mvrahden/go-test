package gotestrunner

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func CleanStaleOverlays() {
	pattern := filepath.Join(os.TempDir(), "gotest-overlay-*")
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if isOverlayAlive(dir) {
			continue
		}
		os.RemoveAll(dir)
	}
}

func isOverlayAlive(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return processAlive(pid)
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
