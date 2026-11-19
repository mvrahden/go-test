package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BuildEventsTestSuite: what one go test -c -json call says is attributed
// to the package it is about.
type BuildEventsTestSuite struct{}

func (s *BuildEventsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// events are go's own lines, as go1.26 and go1.27 print them.
const events = `{"ImportPath":"m/c [m/c.test]","Action":"build-output","Output":"# m/c [m/c.test]\n"}
{"ImportPath":"m/c [m/c.test]","Action":"build-output","Output":"c/x_test.go:5:28: undefined: undefinedThing\n"}
{"ImportPath":"m/c [m/c.test]","Action":"build-fail"}
{"ImportPath":"m/d [m/d.test]","Action":"build-output","Output":"# m/d\n"}
{"ImportPath":"m/d [m/d.test]","Action":"build-output","Output":"# [m/d]\n"}
{"ImportPath":"m/d [m/d.test]","Action":"build-output","Output":"d/x_test.go:8:40: fmt.Printf format %d has arg \"s\" of wrong type string\n"}
{"ImportPath":"m/d [m/d.test]","Action":"build-fail"}
{"ImportPath":"m/g","Action":"build-output","Output":"# m/g\n"}
{"ImportPath":"m/g","Action":"build-output","Output":"g/g.go:3:23: undefined: undefinedG\n"}
{"ImportPath":"m/g","Action":"build-fail"}
go: a plain line
`

func (s *BuildEventsTestSuite) TestEventPackage(t *gotest.T) {
	type row struct{ Desc, In, Want string }
	for t, r := range gotest.Each(t, []row{
		{"a test variant", "m/c [m/c.test]", "m/c"},
		{"an external test package", "m/c_test [m/c.test]", "m/c"},
		{"a dependency rebuilt for a test", "m/q [m/c.test]", "m/c"},
		{"a plain package", "m/g", "m/g"},
	}) {
		gotest.Equal(t, r.Want, gotestrunner.ExportEventPackage(r.In))
	}
}

func (s *BuildEventsTestSuite) TestAttribution(t *gotest.T) {
	log := gotestrunner.ExportNewBuildLog([]string{"m/c", "m/d", "m/e", "m/f"})
	// Fed in two writes that split a line: go's writes are not line-aligned.
	_, _ = log.Write([]byte(events[:137]))
	_, _ = log.Write([]byte(events[137:]))
	t.It("gives a package its compiler report", func(it *gotest.T) {
		gotest.True(it, log.ExportFailed("m/c"))
		gotest.Equal(it, "# m/c [m/c.test]\nc/x_test.go:5:28: undefined: undefinedThing\n", string(log.ExportDiagnostics("m/c")))
	})
	t.It("gives a package its vet finding", func(it *gotest.T) {
		gotest.True(it, log.ExportFailed("m/d"))
		gotest.Contains(it, string(log.ExportDiagnostics("m/d")), `has arg "s" of wrong type string`)
	})
	t.It("holds a dependency's report apart", func(it *gotest.T) {
		gotest.False(it, log.ExportFailed("m/f"))
		gotest.Empty(it, log.ExportDiagnostics("m/f"))
		gotest.Equal(it, "# m/g\ng/g.go:3:23: undefined: undefinedG\ngo: a plain line\n", string(log.ExportShared()))
	})
	t.It("says nothing of a package that built", func(it *gotest.T) {
		gotest.False(it, log.ExportFailed("m/e"))
	})
}
