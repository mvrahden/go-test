package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/mvrahden/go-test/about"
	"golang.org/x/tools/go/packages"
)

//go:embed static
var templates embed.FS

var (
	headerTpl = template.Must(template.New("header").ParseFS(templates, "static/header.*"))
	gotestTpl = template.Must(template.New("gotest").Funcs(tplFuncs).ParseFS(templates, "static/gotest.*"))
	tplFuncs  = map[string]any{}
)

type renderer struct{}

func (r renderer) RenderGoTestSpec(pkgs []*packages.Package, spec SpecOutcome) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkgs, spec); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}
	return buf.Bytes(), nil
}

func (r *renderer) renderFileHeader(buf *bytes.Buffer, pkgs []*packages.Package, spec SpecOutcome) error {
	type Import struct {
		Name string
		Path string
	}
	type TplData struct {
		RepoName    string
		PackageName string
		Imports     []Import
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: strings.TrimSuffix(pkgs[0].Name, "_test") + "_test",
		Imports: []Import{
			{Path: "testing"},
			{Path: about.Repo + "/pkg/gotest"},
			{Path: strings.TrimSuffix(pkgs[0].PkgPath, "_test")},
		},
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}
