package {{.PkgName}}

import (
	"github.com/mvrahden/go-test/pkg/gotest"
{{- range .ExtraImports}}
	"{{.}}"
{{- end}}
)

type {{.SuiteName}} struct{}

func (s *{{.SuiteName}}) Fuzz{{.FuncName}}(f *gotest.F) {
{{.Body}}
}
