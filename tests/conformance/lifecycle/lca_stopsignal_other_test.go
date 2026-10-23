//go:build !unix

package lifecycle_test

import "errors"

// suiteSignallable: Windows has no signal for a single process.
const suiteSignallable = false

func signalSuite(string) error { return errors.New("no signal for a single process") }
