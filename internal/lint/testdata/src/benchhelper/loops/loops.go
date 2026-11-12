package loops

import "github.com/mvrahden/go-test/pkg/gotest"

// Run holds the measured loop in another package.
func Run(b *gotest.B, fn func()) {
	for b.Loop() {
		fn()
	}
}
