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

{{- /* Shared fixture package-level vars */ -}}
{{ range $sf := .SharedFixtureNodes }}
var ƒ_sf_{{ $sf.Identifier }} = &{{ $sf.QualifiedType }}{}
{{ end }}

{{- /* Package fixture package-level vars */ -}}
{{ range $f := .AllFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedType }}
{{ end }}

func TestMain(m *testing.M) {
    var ƒmaxSuiteSetup time.Duration
{{ range $fs := .FlatSuites }}
    {
        ƒscfg := gotest.DefaultSuiteConfig()
{{- if $fs.Suite.HasConfig }}
        gotest.OverlaySuiteConfig(&ƒscfg, (&{{ $fs.Suite.Identifier }}{
{{- range $id, $field := $fs.FixtureFields }}
            {{ $field }}: ƒ_{{ $id }},
{{- end }}
        }).SuiteConfig())
{{- end }}
        if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
    }
{{ end }}

    os.Exit(gotestruntime.RunFixtureMain(m, gotestruntime.MainConfig{
        Fixtures: []*gotestruntime.FixtureNode{
{{- range $sf := .SharedFixtureNodes }}
            {
                Name: "{{ $sf.Identifier }}",
                SharedState: &gotestruntime.SharedStateNode{
                    StateKey: "{{ $sf.StateKey }}",
                    Target: ƒ_sf_{{ $sf.Identifier }},
{{- if $sf.HasHydrate }}
                    Hydrate: func(ctx context.Context) error { return ƒ_sf_{{ $sf.Identifier }}.Hydrate(ctx) },
{{- end }}
{{- if $sf.HasDehydrate }}
                    Dehydrate: func(ctx context.Context) error { return ƒ_sf_{{ $sf.Identifier }}.Dehydrate(ctx) },
{{- end }}
                },
{{- if $sf.ParentFields }}
                Init: func() {
{{- range $parentID, $fieldName := $sf.ParentFields }}
                    ƒ_sf_{{ $sf.Identifier }}.{{ $fieldName }} = ƒ_sf_{{ $parentID }}
{{- end }}
                },
{{- end }}
{{- if $sf.DependsOn }}
                DependsOn: []string{
{{- range $dep := $sf.DependsOn }}
                    "{{ $dep }}",
{{- end }}
                },
{{- end }}
            },
{{- end }}
{{- range $f := .AllFixtures }}
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
{{- range $id, $field := $fs.FixtureFields }}
            {{ $field }}: ƒ_{{ $id }},
{{- end }}
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
{{- range $fix := $fs.FixtureOrder }}
{{- if $fix.AfterEach }}
        defer func() {
            if err := ƒ_{{ $fix.Identifier }}.AfterEach(context.Background()); err != nil {
                it.Errorf("{{ $fix.Identifier }}.AfterEach failed: %v", err)
            }
        }()
{{- end }}
{{- end }}
{{- range $fix := $fs.FixtureOrder }}
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
                    gotest.OverlayFixtureConfig(&cfg, (&{{ .QualifiedType }}{}).FixtureConfig())
                    return cfg
                }(),
{{- else }}
                Config: gotest.DefaultFixtureConfig(),
{{- end }}
                Init: func() {
{{- if or .ParentFieldNames .SharedFixtures }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedType }}{
{{- range $parentID, $fieldName := .ParentFieldNames }}
                        {{ $fieldName }}: ƒ_{{ $parentID }},
{{- end }}
{{- range $sf := .SharedFixtures }}
                        {{ $sf.FieldName }}: ƒ_sf_{{ $sf.Identifier }},
{{- end }}
                    }
{{- else }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedType }}{}
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
{{- if .DependsOn }}
                DependsOn: []string{
{{- range $dep := .DependsOn }}
                    "{{ $dep }}",
{{- end }}
                },
{{- end }}
            },
{{- end -}}
