// Package fuzzharvest is fixture source for RendererTestSuite's harvested-
// seed rendering tests. Unlike testdata/sources (loaded in bulk without
// Tests: true, so _test.go/production splits don't survive), this package
// is loaded directly with Tests: true so it has a REAL production file
// (this one) plus a real _test.go file — the same shape gotestast.HarvestSeeds
// requires to tell test code from production code.
package fuzzharvest

func trimAll(s string) string { return s }
