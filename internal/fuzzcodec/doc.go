// Package fuzzcodec holds the byte codecs generated code uses to fuzz
// arguments Go's fuzzing engine has no type for: a Reader that never rejects
// its input, a Writer that encodes a seed the same way, and the Leaf
// functions for single values. Generated code reaches them through
// pkg/gotestruntime.
package fuzzcodec
