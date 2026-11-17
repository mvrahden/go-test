// Package gotestfuzz holds the byte codecs the harness gotest generates uses
// to fuzz arguments Go's fuzzing engine has no type for: a Reader that never
// rejects its input, a Writer that encodes a seed the same way, and the Leaf
// functions for single values.
//
// It is for generated code only.
package gotestfuzz
