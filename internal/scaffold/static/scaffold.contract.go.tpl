package {{.PkgName}}

import (
	"reflect"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.Name}}ContractTestSuite[T {{.Name}}] struct {
	sut T
}

// BeforeEach builds a fresh implementation per test. A type alias cannot set
// fields, so add one case per instantiated type.
func (s *{{.Name}}ContractTestSuite[T]) BeforeEach(t *gotest.T) {
	switch any(s.sut).(type) {
	// case *MyImpl:
	// 	s.sut = any(NewMyImpl()).(T)
	default:
		gotest.Fail(t, "no constructor for %v: add a case to BeforeEach", reflect.TypeFor[T]())
	}
}
{{range .Methods}}
func (s *{{$.Name}}ContractTestSuite[T]) Test{{.Name}}(t *gotest.T) {
{{- if .ReturnsError}}
	t.It("succeeds", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
	t.It("returns error", func(it *gotest.T) {
		// TODO: test error case
	})
{{- else}}
	t.It("works", func(it *gotest.T) {
		// TODO: test {{$.Name}}.{{.Name}}{{.Signature}}
	})
{{- end}}
}
{{end}}
// Instantiate the contract for a concrete implementation:
// type My{{.Name}}TestSuite = {{.Name}}ContractTestSuite[*MyImpl]
