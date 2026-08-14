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
{{- if $sf.HasConfig }}
var ƒcfg_sf_{{ $sf.Identifier }} gotest.FixtureConfig
{{- end }}
{{ end }}

{{- /* Package fixture package-level vars */ -}}
{{ range $f := .AllFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedType }}
{{- if $f.HasConfig }}
var ƒcfg_{{ $f.Identifier }} gotest.FixtureConfig
{{- end }}
{{ end }}

var ƒ_fixtureOnce gotestruntime.FixtureOnce
var ƒ_fixtureDAG *gotestruntime.FixtureDAG
var ƒ_fixtureTestNames = []string{
{{- range $name := .FixtureTestNames }}
    "{{ $name }}",
{{- end }}
}
var ƒ_pending atomic.Int32

func ƒ_setupFixtures(t testing.TB) {
    if err := ƒ_fixtureOnce.Do(func() error {
{{- /*
  Each config is derived exactly once, but inside ƒ_fixtureOnce.Do rather than at
  package-variable initialisation. A config method that panics has to be
  contained and reported as a setup failure; at package init it would abort the
  binary before TestMain, attributed to nothing. It also has to observe the
  environment TestMain set up, not the one that existed before it ran.
*/}}
{{- range $sf := .SharedFixtureNodes }}
{{- if $sf.HasConfig }}
        ƒcfg_sf_{{ $sf.Identifier }} = ƒ_sf_{{ $sf.Identifier }}.SharedFixtureConfig()
{{- end }}
{{- end }}
{{- range $f := .AllFixtures }}
{{- if $f.HasConfig }}
        ƒcfg_{{ $f.Identifier }} = (&{{ $f.QualifiedType }}{}).FixtureConfig()
{{- end }}
{{- end }}
        ƒ_pending.Store(int32(gotestruntime.CountMatchingTests(ƒ_fixtureTestNames)))
        var ƒmaxSuiteSetup time.Duration
{{ range $fs := .FlatSuites }}
        {
{{- if $fs.Suite.HasConfig }}
            ƒscfg := (&{{ $fs.Suite.Identifier }}{
{{- range $id, $field := $fs.FixtureFields }}
                {{ $field }}: ƒ_{{ $id }},
{{- end }}
            }).SuiteConfig()
{{- else }}
            ƒscfg := gotest.DefaultSuiteConfig()
{{- end }}
            if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
        }
{{ end }}

        var err error
        ƒ_fixtureDAG, err = gotestruntime.SetupFixtureDAG(context.Background(), gotestruntime.MainConfig{
            Fixtures: []*gotestruntime.FixtureNode{
{{- range $sf := .SharedFixtureNodes }}
                {
                    Name: "{{ $sf.Identifier }}",
{{- if $sf.HasConfig }}
                    Config: ƒcfg_sf_{{ $sf.Identifier }},
                    Budget: ƒcfg_sf_{{ $sf.Identifier }}.Timeout,
{{- else }}
                    Config: gotest.DefaultFixtureConfig(),
{{- end }}
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
        })
        return err
    }); err != nil {
        t.Fatalf("fixture setup: %v", err)
    }
    t.Cleanup(func() {
        if ƒ_pending.Add(-1) == 0 {
            if ƒ_fixtureDAG.Teardown() {
                t.Errorf("fixture teardown failed")
            }
        }
    })
}

{{- /* Render fixture-bound suites as top-level Test functions */ -}}
{{ range $fs := .FlatSuites }}

func Test{{ $fs.Suite.Identifier }}(t *testing.T) {
    ƒ_setupFixtures(t)

    s := &ƒƒ_GOTEST_{{ $fs.Suite.Identifier }}{
        {{ $fs.Suite.Identifier }}: {{ $fs.Suite.Identifier }}{
{{- range $id, $field := $fs.FixtureFields }}
            {{ $field }}: ƒ_{{ $id }},
{{- end }}
        },
    }
{{- if $fs.Suite.HasGuard }}
    if ƒreason := s.{{ $fs.Suite.Identifier }}.SuiteGuard(); ƒreason != "" {
        t.Skipf("suite guard: %s", ƒreason)
        return
    }
{{- end }}
{{- if $fs.Suite.HasConfig }}
    ƒcfg := s.{{ $fs.Suite.Identifier }}.SuiteConfig()
    ƒbudget := ƒcfg
{{- else }}
    ƒcfg := gotest.DefaultSuiteConfig()
    ƒbudget := gotest.SuiteConfig{}
{{- end }}
{{- if and $fs.Suite.IsMethodParallel $fs.Suite.TestCases }}
    ƒfailed := &atomic.Bool{}
{{- end }}
{{- /*
  testing runs the suite cleanup only after every subtest started via t.Run has
  finished, parallel ones included. It must never wait on those subtests itself:
  on panic the testing package runs ancestor cleanups from the panicking
  goroutine, so such a wait deadlocks against the panic unwind.
*/}}

    t.Cleanup(func() {
        gotestruntime.RunTeardown(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.AfterAll)
    })
    gotestruntime.RunSetup(t, ƒcfg.SetupTimeout, ƒbudget.SetupTimeout, s.BeforeAll)

{{ range $tc := $fs.Suite.TestCases }}
    t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $fs.Suite.IsMethodParallel }}
        it.Parallel()
        if ƒcfg.FailFast && ƒfailed.Load() {
          it.Skip("FailFast: earlier test failed")
        }
        defer func() { if it.Failed() { ƒfailed.Store(true) } }()
{{- end }}
        ttt := gotestruntime.TestT(it, ƒcfg.Timeout)
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
        gotestruntime.RunTest(ttt, ƒbudget.Timeout, func() {
{{- if $tc.IsAsync }}
          ƒdone := make(chan struct{}, 1)
          s.{{ $tc.Identifier }}({{ if $tc.UsesStdlibT }}ttt.T(){{ else }}ttt{{ end }}, ctx, func() { select { case ƒdone <- struct{}{}: default: } })
          select {
          case <-ƒdone:
          case <-ttt.Context().Done():
            it.Fatalf("%s: done() was not called before the test deadline", "{{ $tc.Identifier }}")
          }
{{- else }}
          s.{{ $tc.Identifier }}({{ if $tc.UsesStdlibT }}ttt.T(){{ else }}ttt{{ end }}, ctx)
{{- end }}
        })
{{- else }}
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        gotestruntime.RunTest(ttt, ƒbudget.Timeout, func() {
{{- if $tc.IsAsync }}
          ƒdone := make(chan struct{}, 1)
          s.{{ $tc.Identifier }}({{ if $tc.UsesStdlibT }}ttt.T(){{ else }}ttt{{ end }}, func() { select { case ƒdone <- struct{}{}: default: } })
          select {
          case <-ƒdone:
          case <-ttt.Context().Done():
            it.Fatalf("%s: done() was not called before the test deadline", "{{ $tc.Identifier }}")
          }
{{- else }}
          ƒƒ_GOTEST_exec({{ if $tc.UsesStdlibT }}func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }{{ else }}s.{{ $tc.Identifier }}{{ end }}, ttt)
{{- end }}
        })
{{- end }}
    })
{{- if not $fs.Suite.IsMethodParallel }}
    if ƒcfg.FailFast && t.Failed() {
        return
    }
{{- end }}
{{ end }}
}
{{ end }}

{{- define "fixtureNode" -}}
            {
                Name: "{{ .Identifier }}",
{{- if .HasConfig }}
                Config: ƒcfg_{{ .Identifier }},
                Budget: ƒcfg_{{ .Identifier }}.Timeout,
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
