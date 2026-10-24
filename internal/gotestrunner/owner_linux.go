package gotestrunner

import (
	"os"
	"strings"
)

// pidNamespace names this process's pid namespace.
func pidNamespace() string { return readPIDNamespace("/proc/self/ns/pid") }

// readPIDNamespace reads the pid namespace link, or is unknownID.
func readPIDNamespace(link string) string {
	ns, err := os.Readlink(link)
	if err != nil {
		return unknownID
	}
	return ns
}

// bootID names this host's current boot.
func bootID() string { return readBootID("/proc/sys/kernel/random/boot_id") }

// readBootID reads a boot id file, or is unknownID.
func readBootID(path string) string {
	data, err := os.ReadFile(path)
	if id := strings.TrimSpace(string(data)); err == nil && id != "" {
		return id
	}
	return unknownID
}
