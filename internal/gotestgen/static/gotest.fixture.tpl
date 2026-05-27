{{- /* Declare wrapper structs for all fixture-bound suites at file scope */ -}}
{{ range $ts := .FixtureBoundSuites }}

type ƒƒ_GOTEST_{{ $ts.Identifier }} struct {
  {{ $ts.Identifier }}
}

func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeAll(it *gotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll({{ if $ts.BeforeAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterAll(it *gotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll({{ if $ts.AfterAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- if $ts.HasReturningBeforeEach }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) {{ $ts.ContextTypeName }} { {{ if $ts.BeforeEach -}} return ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ else }}return nil {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T, ctx {{ $ts.ContextTypeName }}) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}, ctx) {{ end }}}
{{- else }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- end }}
{{- end }}

{{- /* Package-level fixture variables for all fixtures in the tree */ -}}
{{ range $f := .AllFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedIdentifier }}
{{- range $sf := $f.SharedFixtures }}
var ƒ_{{ $sf.LocalVar }}_{{ $f.Identifier }} = &{{ $sf.QualifiedType }}{}
{{- end }}
{{ end }}

func TestMain(m *testing.M) {
    var ƒmaxSuiteSetup time.Duration
{{ range $fs := .FlatSuites }}
    {
        ƒscfg := gotest.DefaultSuiteConfig()
{{- if $fs.Suite.HasConfig }}
        gotest.OverlaySuiteConfig(&ƒscfg, (&{{ $fs.Suite.Identifier }}{ {{ $fs.Suite.FixtureFieldName }}: ƒ_{{ $fs.Fixture.Identifier }} }).SuiteConfig())
{{- end }}
        if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
    }
{{ end }}

    os.Exit(gotestruntime.RunFixtureMain(m, gotestruntime.MainConfig{
        Roots: []*gotestruntime.FixtureNode{
{{- range $f := .RootFixtures }}
{{ template "fixtureNode" $f }}
{{- end }}
        },
        MaxSuiteSetupTimeout: ƒmaxSuiteSetup,
    }))
}

{{- /* Render fixture-bound suites as top-level Test functions */ -}}
{{ range $fs := .FlatSuites }}

func Test{{ $fs.Suite.Identifier }}(t *testing.T) {
    s := &ƒƒ_GOTEST_{{ $fs.Suite.Identifier }}{
        {{ $fs.Suite.Identifier }}: {{ $fs.Suite.Identifier }}{
            {{ $fs.Suite.FixtureFieldName }}: ƒ_{{ $fs.Fixture.Identifier }},
        },
    }
    ƒcfg := gotest.DefaultSuiteConfig()
{{- if $fs.Suite.HasConfig }}
    gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $fs.Suite.Identifier }}.SuiteConfig())
{{- end }}

{{- if $fs.Suite.IsMethodParallel }}
    wg := &sync.WaitGroup{}
{{- end }}

    ƒsetupT := gotest.NewT(t)
    if ƒcfg.SetupTimeout > 0 {
        ƒsetupT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
    }
    t.Cleanup(func() {
{{- if $fs.Suite.IsMethodParallel }}
        wg.Wait()
{{- end }}
        ƒteardownT := gotest.NewT(t)
        if ƒcfg.SetupTimeout > 0 {
            ƒteardownT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
        }
        s.AfterAll(ƒteardownT)
    })
    s.BeforeAll(ƒsetupT)

{{ range $tc := $fs.Suite.TestCases }}
    t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $fs.Suite.IsMethodParallel }}
        wg.Add(1)
        it.Parallel()
        defer wg.Done()
{{- end }}
        ttt := gotest.NewT(it)
        if ƒcfg.Timeout > 0 {
            ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
        }
{{- range $fix := $fs.FixtureChain }}
{{- if $fix.AfterEach }}
        defer func() {
            if err := ƒ_{{ $fix.Identifier }}.AfterEach(context.Background()); err != nil {
                it.Errorf("{{ $fix.Identifier }}.AfterEach failed: %v", err)
            }
        }()
{{- end }}
{{- end }}
{{- range $fix := $fs.FixtureChain }}
{{- if $fix.BeforeEach }}
        if err := ƒ_{{ $fix.Identifier }}.BeforeEach(it.Context()); err != nil {
            it.Fatalf("{{ $fix.Identifier }}.BeforeEach failed: %v", err)
        }
{{- end }}
{{- end }}
{{- if $fs.Suite.HasReturningBeforeEach }}
        ctx := s.BeforeEach(ttt)
        defer s.AfterEach(ttt, ctx)
        s.{{ $tc.Identifier }}({{ if $tc.UsesStdlibT }}ttt.T(){{ else }}ttt{{ end }}, ctx)
{{- else }}
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        ƒƒ_GOTEST_exec({{ if $tc.UsesStdlibT }}func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }{{ else }}s.{{ $tc.Identifier }}{{ end }}, ttt)
{{- end }}
    })
    if ƒcfg.FailFast && t.Failed() {
        return
    }
{{ end }}
}
{{ end }}

{{- define "fixtureNode" -}}
            {
                Name: "{{ .Identifier }}",
{{- if .HasConfig }}
                Config: func() gotest.FixtureConfig {
                    cfg := gotest.DefaultFixtureConfig()
                    gotest.OverlayFixtureConfig(&cfg, (&{{ .QualifiedIdentifier }}{}).FixtureConfig())
                    return cfg
                }(),
{{- else }}
                Config: gotest.DefaultFixtureConfig(),
{{- end }}
                Init: func() {
{{- if .ParentIdentifier }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedIdentifier }}{
                        {{ .ParentFieldName }}: ƒ_{{ .ParentIdentifier }},
                    }
{{- else }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedIdentifier }}{}
{{- end }}
                },
                BeforeAll: func(ctx context.Context) error {
                    return ƒ_{{ .Identifier }}.BeforeAll(ctx)
                },
{{- if .AfterAll }}
                AfterAll: func(ctx context.Context) error {
                    return ƒ_{{ .Identifier }}.AfterAll(ctx)
                },
{{- end }}
{{- if .SharedFixtures }}
                SharedFixtures: []gotestruntime.SharedFixtureBinding{
{{- range $sf := .SharedFixtures }}
                    {
                        StateKey: "{{ $sf.StateKey }}",
                        Target: ƒ_{{ $sf.LocalVar }}_{{ $.Identifier }},
{{- if $sf.HasHydrate }}
                        Hydrate: func(ctx context.Context) error { return ƒ_{{ $sf.LocalVar }}_{{ $.Identifier }}.Hydrate(ctx) },
{{- end }}
{{- if $sf.HasDehydrate }}
                        Dehydrate: func(ctx context.Context) error { return ƒ_{{ $sf.LocalVar }}_{{ $.Identifier }}.Dehydrate(ctx) },
{{- end }}
                        Assign: func() { ƒ_{{ $.Identifier }}.{{ $sf.FieldName }} = ƒ_{{ $sf.LocalVar }}_{{ $.Identifier }} },
                    },
{{- end }}
                },
{{- end }}
{{- if .ChildFixtures }}
                Children: []*gotestruntime.FixtureNode{
{{- range $child := .ChildFixtures }}
{{ template "fixtureNode" $child }}
{{- end }}
                },
{{- end }}
            },
{{- end -}}
