package stdlibonly

import "testing"

// TestPlain is a stdlib test: gotest runs suites, so it must say that this
// one was not run instead of reporting an empty green package.
func TestPlain(t *testing.T) {}
