//go:build !windows

package testkit

import "os"

func renameDir(from, to string) error { return os.Rename(from, to) }
