//go:build ignore

// Command gen is built alone with go run, as package main.
package main

type UnresolvedTestSuite struct{}

func main() { _ = UnresolvedTestSuite{} }
