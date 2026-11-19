// Package docref renders the parts of the documents that restate code: the
// assertions of pkg/gotest, the fields and presets of its config types,
// gotest's own flags and the environment variables a user may set.
//
// A document marks each such part with a pair of lines,
//
//	<!-- gotest:generated <section> <format> -->
//	<!-- gotest:end -->
//
// and everything between them is replaced by the section's rendering in that
// format. Text outside the pairs is never touched. `make reference` rewrites
// the documents; a conventions suite fails on a section that differs from its
// rendering.
package docref
