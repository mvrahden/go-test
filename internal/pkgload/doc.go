// Package pkgload loads packages with their tests, type-checked from source,
// without making the go command build a test variant or a test main.
//
// go/packages asks for types through `go list -export`, which compiles every
// package it lists, the test variants and the test mains included, and then
// type-checks the same packages again from source. Load lists the graph
// without export data, asks for export data only for the plain packages the
// test packages import, and type-checks the rest itself.
package pkgload
