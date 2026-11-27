// Package c embeds a file.
package c

import _ "embed"

// Data is data.txt.
//
//go:embed data.txt
var Data string
