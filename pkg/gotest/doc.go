// Package gotest is what a gotest suite is written against: the T, B and F
// its methods take, the assertions, polls and snapshots they check with, and
// the SuiteConfig and FixtureConfig a suite or a fixture declares.
//
// A suite is a struct whose name ends in TestSuite, with Test, Benchmark and
// Fuzz methods on its pointer and optional BeforeAll, AfterAll, BeforeEach
// and AfterEach hooks. Nothing in this package runs one: the gotest command
// generates the harness that does. Run suites with
//
//	go tool gotest ./...
//
// Plain go test finds no test in a package of suites and reports it ok.
//
// An assertion takes a *T, *B, *F, *R or *testing.T, reports its failure at
// the line that failed and ends the test there. What each one promises is
// stated in the specs directory of the repository.
package gotest
