//go:build windows

package gotestrunner_test

import "os"

var gracefulSignals = []os.Signal{os.Interrupt}
