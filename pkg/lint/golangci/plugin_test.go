package golangci_test

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/lint/golangci"
)

func TestPlugin(t *testing.T) {
	newPlugin, err := register.GetPlugin("gotestlint")
	gotest.NoError(t, err)
	p, err := newPlugin(map[string]any{"skip": []any{"testify"}, "nolint-allow": []any{"nilaway"}})
	gotest.NoError(t, err)
	analyzers, err := p.BuildAnalyzers()
	gotest.NoError(t, err)
	gotest.Len(t, analyzers, 1)
	gotest.Equal(t, "gotestlint", analyzers[0].Name)
	gotest.Equal(t, register.LoadModeTypesInfo, p.GetLoadMode())
	analysistest.Run(t, analysistest.TestData(), analyzers[0], "probe")
}

func TestRefusedSettings(t *testing.T) {
	_, err := golangci.New(map[string]any{"skip": []any{"focus"}})
	gotest.ErrorContains(t, err, `cannot skip "focus"`)
	_, err = golangci.New(map[string]any{"skipp": []any{}})
	gotest.Error(t, err)
	_, err = golangci.New(map[string]any{})
	gotest.NoError(t, err)
	_, err = golangci.New(nil)
	gotest.NoError(t, err)
}
