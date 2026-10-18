package gotestruntime

import (
	"io"
	"time"
)

// ExportSharedLifetime is a SharedLifetime that writes to stderr, ends the
// process through exit and waits through sleep, adding headroom to its
// fixtures' teardown budgets.
func ExportSharedLifetime(stderr io.Writer, exit func(int), sleep func(time.Duration), headroom time.Duration, names ...string) *SharedLifetime {
	return newSharedLifetime(stderr, exit, sleep, headroom, names)
}
