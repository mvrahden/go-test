package gotestgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"strings"
	"text/template"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

//go:embed static
var templates embed.FS

var (
	headerTpl = template.Must(template.New("header").ParseFS(templates, "static/header.*"))
	gotestTpl = template.Must(template.New("gotest").Funcs(tplFuncs).ParseFS(templates, "static/gotest.*"))
	tplFuncs  = map[string]any{
		"hasSuffix": strings.HasSuffix,
	}
)

type renderer struct{}

func (r renderer) RenderGoTestSpec(pkgs []*packages.Package, spec SpecOutcome) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := r.renderFileHeader(buf, pkgs, spec); err != nil {
		return nil, fmt.Errorf("failed rendering file header. err: %w", err)
	}
	if err := r.renderTestSuites(buf, pkgs, spec); err != nil {
		return nil, fmt.Errorf("failed rendering test suites. err: %w", err)
	}
	return r.formatOutput(buf)
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
	imports := []Import{
		{Path: "testing"},
		{Path: about.Repo + "/pkg/gotest"},
		{Path: strings.TrimSuffix(pkgs[0].PkgPath, "_test")},
	}
	if slices.Any(spec.EffectiveTestSuites, func(v *gotestast.TestSuiteSpec, idx int) bool {
		return v.HasParallelTestCases()
	}) {
		imports = append(imports, Import{Path: "sync"})
	}
	data := TplData{
		RepoName:    about.ShortInfo(),
		PackageName: strings.TrimSuffix(pkgs[0].Name, "_test") + "_test",
		Imports:     imports,
	}
	return headerTpl.ExecuteTemplate(buf, "header.go.tpl", map[string]any{"Header": data})
}

func (r *renderer) renderTestSuites(buf *bytes.Buffer, pkgs []*packages.Package, spec SpecOutcome) error {
	type TplData struct{}
	data := TplData{}
	return gotestTpl.ExecuteTemplate(buf, "gotest.suites.tpl", map[string]any{
		"Spec": spec,
		"Data": data,
	})
}

func (renderer) formatOutput(buf *bytes.Buffer) ([]byte, error) {
	srcs, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed formatting the generated sources. err: %w", err)
	}
	return srcs, nil
}
