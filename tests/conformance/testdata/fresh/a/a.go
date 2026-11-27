// Package a imports b.
package a

import "fresh/b"

// Two is b's One twice.
func Two() int { return 2 * b.One() }
