package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.Name}}ContractTestSuite[T {{.Name}}] struct {
	factory func() T
	sut     T
}

func (s *{{.Name}}ContractTestSuite[T]) BeforeEach(t *gotest.T) {
	s.sut = s.factory()
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
