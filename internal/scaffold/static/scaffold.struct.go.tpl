package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.Name}}TestSuite struct {
	sut *{{.Name}}
}

func (s *{{.Name}}TestSuite) BeforeEach(t *gotest.T) {
	s.sut = nil // TODO: initialize {{.Name}}
}
{{range .Methods}}
func (s *{{$.Name}}TestSuite) Test{{.Name}}(t *gotest.T) {
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