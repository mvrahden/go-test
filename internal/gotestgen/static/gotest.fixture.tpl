{{- /* Declare wrapper structs for all fixture-bound suites at file scope */ -}}
{{ range $ts := .FixtureBoundSuites }}

type ƒƒ_GOTEST_{{ $ts.Identifier }} struct {
  {{ $ts.Identifier }}
}

func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeAll(it *gotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll({{ if $ts.BeforeAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterAll(it *gotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll({{ if $ts.AfterAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- end }}

func TestMain(m *testing.M) { os.Exit(m.Run()) }

{{ range $f := .RootFixtures }}
func Test_{{ $f.Identifier }}(t *testing.T) {
{{- /* Resolve shared fixture embeddings from JSON state */ -}}
{{ if $f.SharedFixtures }}
    ƒsharedFile := os.Getenv("GOTEST_SHARED_STATE_FILE")
    if ƒsharedFile == "" {
        t.Fatal("GOTEST_SHARED_STATE_FILE not set — run via gotest CLI")
    }
    ƒsharedRaw, ƒsharedErr := os.ReadFile(ƒsharedFile)
    if ƒsharedErr != nil {
        t.Fatalf("read shared state file: %v", ƒsharedErr)
    }
    ƒsharedState := map[string]json.RawMessage{}
    if err := json.Unmarshal(ƒsharedRaw, &ƒsharedState); err != nil {
        t.Fatalf("unmarshal shared state: %v", err)
    }
{{ range $sf := $f.SharedFixtures }}
    {{ $sf.LocalVar }} := &{{ $sf.QualifiedType }}{}
    if ƒb, ok := ƒsharedState["{{ $sf.StateKey }}"]; ok {
        if err := json.Unmarshal(ƒb, {{ $sf.LocalVar }}); err != nil {
            t.Fatalf("unmarshal {{ $sf.FieldName }} state: %v", err)
        }
    }
{{- if $sf.HasHydrate }}
    if err := {{ $sf.LocalVar }}.Hydrate(context.Background()); err != nil {
        t.Fatalf("{{ $sf.FieldName }}.Hydrate: %v", err)
    }
{{- end }}
{{- if $sf.HasDehydrate }}
    t.Cleanup(func() { {{ $sf.LocalVar }}.Dehydrate(context.Background()) })
{{- end }}
{{ end }}
{{ end }}

    fixture := &{{ $f.QualifiedIdentifier }}{}
{{- range $sf := $f.SharedFixtures }}
    fixture.{{ $sf.FieldName }} = {{ $sf.LocalVar }}
{{- end }}
    ƒcfg := gotest.DefaultFixtureConfig()
{{- if $f.HasConfig }}
    gotest.OverlayFixtureConfig(&ƒcfg, fixture.FixtureConfig())
{{- end }}
    t.Cleanup(func() {
{{- if $f.AfterAll }}
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        if err := fixture.AfterAll(ctx); err != nil {
            t.Errorf("{{ $f.Identifier }}.AfterAll failed: %v", err)
        }
{{- end }}
    })
    var ƒerr error
    ƒattempts := 1 + ƒcfg.Retries
    for ƒi := range ƒattempts {
        ctx := t.Context()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        ƒerr = fixture.BeforeAll(ctx)
        if ƒerr == nil {
            break
        }
        if ƒi < ƒattempts-1 {
            t.Logf("{{ $f.Identifier }}.BeforeAll attempt %d/%d failed: %v", ƒi+1, ƒattempts, ƒerr)
            if ƒcfg.RetryDelay > 0 {
                time.Sleep(ƒcfg.RetryDelay)
            }
        }
    }
    if ƒerr != nil {
        t.Fatalf("{{ $f.Identifier }}.BeforeAll failed after %d attempt(s): %v", ƒattempts, ƒerr)
    }

{{- /* Render child suites */ -}}
{{ range $ts := $f.ChildSuites }}
    t.Run("{{ $ts.Identifier }}", func(t *testing.T) {
        s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
            {{ $ts.Identifier }}: {{ $ts.Identifier }}{
                {{ $ts.FixtureFieldName }}: fixture,
            },
        }
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
        t.Parallel()
{{- end }}
        ƒscfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
        gotest.OverlaySuiteConfig(&ƒscfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{ if $ts.TestCases -}}
        newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
            return func(tt *gotest.T) {
                t := tt.T()
                t.Run(desc, func(it *testing.T) {
                    ttt := gotest.NewT(it)
                    if ƒscfg.Timeout > 0 {
                        ttt = gotest.NewTWithDeadline(it, ƒscfg.Timeout)
                    }
{{- if $f.AfterEach }}
                    defer func() {
                        if err := fixture.AfterEach(context.Background()); err != nil {
                            it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
                        }
                    }()
{{- end }}
{{- if $f.BeforeEach }}
                    if err := fixture.BeforeEach(it.Context()); err != nil {
                        it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
                    }
{{- end }}
                    defer s.AfterEach(ttt)
                    s.BeforeEach(ttt)
                    ƒƒ_GOTEST_exec(testFn, ttt)
                })
            }}
{{- end }}
{{- if $ts.HasParallelTestCases }}
        newParallelTestCase := func(desc string, wg *sync.WaitGroup, testFn gotest.TestCase) gotest.TestCase {
            wg.Add(1)
            return func(tt *gotest.T) {
                t := tt.T()
                t.Run(desc, func(it *testing.T) {
                    it.Parallel()
                    defer wg.Done()
                    ttt := gotest.NewT(it)
                    if ƒscfg.Timeout > 0 {
                        ttt = gotest.NewTWithDeadline(it, ƒscfg.Timeout)
                    }
{{- if $f.AfterEach }}
                    defer func() {
                        if err := fixture.AfterEach(context.Background()); err != nil {
                            it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
                        }
                    }()
{{- end }}
{{- if $f.BeforeEach }}
                    if err := fixture.BeforeEach(it.Context()); err != nil {
                        it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
                    }
{{- end }}
                    defer s.AfterEach(ttt)
                    s.BeforeEach(ttt)
                    ƒƒ_GOTEST_exec(testFn, ttt)
                })
            }}
        wg := &sync.WaitGroup{}
{{- end }}

        testCases := []gotest.TestCase{
{{- range $tc := $ts.TestCases }}
  {{- if not $tc.IsParallel }}
    {{- if $tc.UsesStdlibT }}
            newTestCase("{{ $tc.Identifier }}", func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }),
    {{- else }}
            newTestCase("{{ $tc.Identifier }}", s.{{ $tc.Identifier }}),
    {{- end }}
  {{- else }}
    {{- if $tc.UsesStdlibT }}
            newParallelTestCase("{{ $tc.Identifier }}", wg, func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }),
    {{- else }}
            newParallelTestCase("{{ $tc.Identifier }}", wg, s.{{ $tc.Identifier }}),
    {{- end }}
  {{- end }}
{{- end }}
        }

        tt := gotest.NewT(t)
        t.Cleanup(func() {
{{- if $ts.HasParallelTestCases }}
            wg.Wait()
{{- end }}
            s.AfterAll(tt)
        })
        s.BeforeAll(tt)
        for _, tc := range testCases {
            tc(tt)
            if ƒscfg.FailFast && t.Failed() {
                break
            }
        }
    })
{{ end }}

{{- /* Render child fixtures (Level 2 nesting) */ -}}
{{ range $cf := $f.ChildFixtures }}
    t.Run("{{ $cf.Identifier }}", func(t *testing.T) {
        child := &{{ $cf.QualifiedIdentifier }}{
            {{ $cf.ParentFieldName }}: fixture,
        }
        ƒcfg_child := gotest.DefaultFixtureConfig()
{{- if $cf.HasConfig }}
        gotest.OverlayFixtureConfig(&ƒcfg_child, child.FixtureConfig())
{{- end }}
        t.Cleanup(func() {
{{- if $cf.AfterAll }}
            ctx := context.Background()
            if ƒcfg_child.Timeout > 0 {
                var cancel context.CancelFunc
                ctx, cancel = context.WithTimeout(ctx, ƒcfg_child.Timeout)
                defer cancel()
            }
            if err := child.AfterAll(ctx); err != nil {
                t.Errorf("{{ $cf.Identifier }}.AfterAll failed: %v", err)
            }
{{- end }}
        })
        var ƒerr_child error
        ƒattempts_child := 1 + ƒcfg_child.Retries
        for ƒi := range ƒattempts_child {
            ctx := t.Context()
            if ƒcfg_child.Timeout > 0 {
                var cancel context.CancelFunc
                ctx, cancel = context.WithTimeout(ctx, ƒcfg_child.Timeout)
                defer cancel()
            }
            ƒerr_child = child.BeforeAll(ctx)
            if ƒerr_child == nil {
                break
            }
            if ƒi < ƒattempts_child-1 {
                t.Logf("{{ $cf.Identifier }}.BeforeAll attempt %d/%d failed: %v", ƒi+1, ƒattempts_child, ƒerr_child)
                if ƒcfg_child.RetryDelay > 0 {
                    time.Sleep(ƒcfg_child.RetryDelay)
                }
            }
        }
        if ƒerr_child != nil {
            t.Fatalf("{{ $cf.Identifier }}.BeforeAll failed after %d attempt(s): %v", ƒattempts_child, ƒerr_child)
        }

{{- range $ts := $cf.ChildSuites }}
        t.Run("{{ $ts.Identifier }}", func(t *testing.T) {
            s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
                {{ $ts.Identifier }}: {{ $ts.Identifier }}{
                    {{ $ts.FixtureFieldName }}: child,
                },
            }
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
            t.Parallel()
{{- end }}
            ƒscfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
            gotest.OverlaySuiteConfig(&ƒscfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{ if $ts.TestCases -}}
            newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
                return func(tt *gotest.T) {
                    t := tt.T()
                    t.Run(desc, func(it *testing.T) {
                        ttt := gotest.NewT(it)
                        if ƒscfg.Timeout > 0 {
                            ttt = gotest.NewTWithDeadline(it, ƒscfg.Timeout)
                        }
{{- if $f.AfterEach }}
                        defer func() {
                            if err := fixture.AfterEach(context.Background()); err != nil {
                                it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
                            }
                        }()
{{- end }}
{{- if $f.BeforeEach }}
                        if err := fixture.BeforeEach(it.Context()); err != nil {
                            it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
                        }
{{- end }}
{{- if $cf.AfterEach }}
                        defer func() {
                            if err := child.AfterEach(context.Background()); err != nil {
                                it.Errorf("{{ $cf.Identifier }}.AfterEach failed: %v", err)
                            }
                        }()
{{- end }}
{{- if $cf.BeforeEach }}
                        if err := child.BeforeEach(it.Context()); err != nil {
                            it.Fatalf("{{ $cf.Identifier }}.BeforeEach failed: %v", err)
                        }
{{- end }}
                        defer s.AfterEach(ttt)
                        s.BeforeEach(ttt)
                        ƒƒ_GOTEST_exec(testFn, ttt)
                    })
                }}
{{- end }}
{{- if $ts.HasParallelTestCases }}
            newParallelTestCase := func(desc string, wg *sync.WaitGroup, testFn gotest.TestCase) gotest.TestCase {
                wg.Add(1)
                return func(tt *gotest.T) {
                    t := tt.T()
                    t.Run(desc, func(it *testing.T) {
                        it.Parallel()
                        defer wg.Done()
                        ttt := gotest.NewT(it)
                        if ƒscfg.Timeout > 0 {
                            ttt = gotest.NewTWithDeadline(it, ƒscfg.Timeout)
                        }
{{- if $f.AfterEach }}
                        defer func() {
                            if err := fixture.AfterEach(context.Background()); err != nil {
                                it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
                            }
                        }()
{{- end }}
{{- if $f.BeforeEach }}
                        if err := fixture.BeforeEach(it.Context()); err != nil {
                            it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
                        }
{{- end }}
{{- if $cf.AfterEach }}
                        defer func() {
                            if err := child.AfterEach(context.Background()); err != nil {
                                it.Errorf("{{ $cf.Identifier }}.AfterEach failed: %v", err)
                            }
                        }()
{{- end }}
{{- if $cf.BeforeEach }}
                        if err := child.BeforeEach(it.Context()); err != nil {
                            it.Fatalf("{{ $cf.Identifier }}.BeforeEach failed: %v", err)
                        }
{{- end }}
                        defer s.AfterEach(ttt)
                        s.BeforeEach(ttt)
                        ƒƒ_GOTEST_exec(testFn, ttt)
                    })
                }}
            wg := &sync.WaitGroup{}
{{- end }}

            testCases := []gotest.TestCase{
{{- range $tc := $ts.TestCases }}
  {{- if not $tc.IsParallel }}
    {{- if $tc.UsesStdlibT }}
                newTestCase("{{ $tc.Identifier }}", func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }),
    {{- else }}
                newTestCase("{{ $tc.Identifier }}", s.{{ $tc.Identifier }}),
    {{- end }}
  {{- else }}
    {{- if $tc.UsesStdlibT }}
                newParallelTestCase("{{ $tc.Identifier }}", wg, func(t *gotest.T) { s.{{ $tc.Identifier }}(t.T()) }),
    {{- else }}
                newParallelTestCase("{{ $tc.Identifier }}", wg, s.{{ $tc.Identifier }}),
    {{- end }}
  {{- end }}
{{- end }}
            }

            tt := gotest.NewT(t)
            t.Cleanup(func() {
{{- if $ts.HasParallelTestCases }}
                wg.Wait()
{{- end }}
                s.AfterAll(tt)
            })
            s.BeforeAll(tt)
            for _, tc := range testCases {
                tc(tt)
                if ƒscfg.FailFast && t.Failed() {
                    break
                }
            }
        })
{{- end }}
    })
{{ end }}
}
{{ end -}}
