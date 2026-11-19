// Package inpkg has in-package tests only.
package inpkg

import "strings"

// Shout upper-cases s.
func Shout(s string) string { return strings.ToUpper(s) + "!" }
