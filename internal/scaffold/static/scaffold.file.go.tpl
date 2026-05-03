package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type {{.SuiteName}} struct {
	gotest.TestSuite
}
{{range .Funcs}}
func (s *{{$.SuiteName}}) Test{{.Name}}(t *gotest.T) {
	t.It("works", func(it *gotest.T) {
		// TODO: test {{.Name}}{{.Signature}}
	})
}
{{end}}