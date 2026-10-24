//go:build !linux

package gotestrunner

import "os"

// pidNamespace is empty: only Linux has pid namespaces.
func pidNamespace() string { return "" }

// bootID names this host: its boot cannot be read portably, and a pid is
// judged only on the host that wrote it.
func bootID() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return unknownID
}
