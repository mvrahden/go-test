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

{{- /* Package-level fixture variables */ -}}
{{ range $f := .RootFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedIdentifier }}
{{ range $cf := $f.ChildFixtures }}
var ƒ_{{ $cf.Identifier }} *{{ $cf.QualifiedIdentifier }}
{{ end }}
{{ end }}

{{- /*
Go flushes coverage counters inside m.Run(), before fixture AfterAll runs.
We swap tearDown with a no-op so counters survive, then flush after teardown.
The linkname struct layout is guarded by TestLinknameCompat tests.
*/}}
// ƒflushCoverage triggers coverage report writing.
//go:linkname ƒflushCoverage testing.coverReport
func ƒflushCoverage()

// ƒtestingCover exposes the stdlib coverage state for tearDown interception.
//go:linkname ƒtestingCover testing.{{ .CoverVarName }}
var ƒtestingCover struct {
    mode        string
    tearDown    func(coverprofile string, gocoverdir string) (string, error)
    snapshotcov func() float64
}

func TestMain(m *testing.M) { os.Exit(ƒƒ_GOTEST_main(m)) }

func ƒƒ_GOTEST_main(m *testing.M) (ƒcode int) {
{{ range $f := .RootFixtures }}
    var ƒcfg_{{ $f.Identifier }} gotest.FixtureConfig
    var ƒok_{{ $f.Identifier }} bool
{{ range $cf := $f.ChildFixtures }}
    var ƒcfg_{{ $cf.Identifier }} gotest.FixtureConfig
    var ƒok_{{ $cf.Identifier }} bool
{{ end }}
{{ end }}

    var ƒorigTearDown func(string, string) (string, error)
    if testing.CoverMode() != "" {
        ƒorigTearDown = ƒtestingCover.tearDown
        ƒtestingCover.tearDown = func(string, string) (string, error) { return "", nil }
    }

    defer func() {
        var ƒtwg sync.WaitGroup
        ƒtdfails := make([]bool, {{ len .RootFixtures }})
{{ range $i, $f := .RootFixtures }}
        ƒtwg.Add(1)
        go func() {
            defer ƒtwg.Done()
{{ if $f.ChildFixtures }}
            var ƒcwg sync.WaitGroup
            ƒcdfails := make([]bool, {{ len $f.ChildFixtures }})
{{ range $j, $cf := $f.ChildFixtures }}
{{- if $cf.AfterAll }}
            if ƒok_{{ $cf.Identifier }} {
                ƒcwg.Add(1)
                go func() {
                    defer ƒcwg.Done()
                    ctx := context.Background()
                    if ƒcfg_{{ $cf.Identifier }}.Timeout > 0 {
                        var cancel context.CancelFunc
                        ctx, cancel = context.WithTimeout(ctx, ƒcfg_{{ $cf.Identifier }}.Timeout)
                        defer cancel()
                    }
                    if err := ƒ_{{ $cf.Identifier }}.AfterAll(ctx); err != nil {
                        fmt.Fprintf(os.Stderr, "{{ $cf.Identifier }}.AfterAll failed: %v\n", err)
                        ƒcdfails[{{ $j }}] = true
                    }
                }()
            }
{{- end }}
{{ end }}
            ƒcwg.Wait()
            for _, ƒf := range ƒcdfails {
                if ƒf { ƒtdfails[{{ $i }}] = true; break }
            }
{{ end }}
{{- if $f.AfterAll }}
            if ƒok_{{ $f.Identifier }} {
                ctx := context.Background()
                if ƒcfg_{{ $f.Identifier }}.Timeout > 0 {
                    var cancel context.CancelFunc
                    ctx, cancel = context.WithTimeout(ctx, ƒcfg_{{ $f.Identifier }}.Timeout)
                    defer cancel()
                }
                if err := ƒ_{{ $f.Identifier }}.AfterAll(ctx); err != nil {
                    fmt.Fprintf(os.Stderr, "{{ $f.Identifier }}.AfterAll failed: %v\n", err)
                    ƒtdfails[{{ $i }}] = true
                }
            }
{{- end }}
{{- range $sf := $f.SharedFixtures }}
{{- if $sf.HasDehydrate }}
            if ƒok_{{ $f.Identifier }} {
                ƒ_{{ $f.Identifier }}.{{ $sf.FieldName }}.Dehydrate(context.Background())
            }
{{- end }}
{{- end }}
        }()
{{ end }}
        ƒtwg.Wait()
        for _, ƒf := range ƒtdfails {
            if ƒf && ƒcode == 0 { ƒcode = 1; break }
        }
        if ƒorigTearDown != nil {
            ƒtestingCover.tearDown = ƒorigTearDown
            ƒflushCoverage()
        }
    }()

    ƒctx, ƒcancel := context.WithCancel(context.Background())
    defer ƒcancel()

    {
        var ƒswg sync.WaitGroup
        ƒtfails := make([]bool, {{ len .RootFixtures }})
{{ range $i, $f := .RootFixtures }}
        ƒswg.Add(1)
        go func() {
            defer ƒswg.Done()
{{ if $f.SharedFixtures }}
            ƒsharedFile := os.Getenv("GOTEST_SHARED_STATE_FILE")
            if ƒsharedFile == "" {
                fmt.Fprintln(os.Stderr, "FAIL: GOTEST_SHARED_STATE_FILE not set — run via gotest CLI")
                ƒtfails[{{ $i }}] = true
                ƒcancel()
                return
            }
            ƒsharedRaw, ƒsharedErr := os.ReadFile(ƒsharedFile)
            if ƒsharedErr != nil {
                fmt.Fprintf(os.Stderr, "FAIL: read shared state file: %v\n", ƒsharedErr)
                ƒtfails[{{ $i }}] = true
                ƒcancel()
                return
            }
            ƒsharedState := map[string]json.RawMessage{}
            if err := json.Unmarshal(ƒsharedRaw, &ƒsharedState); err != nil {
                fmt.Fprintf(os.Stderr, "FAIL: unmarshal shared state: %v\n", err)
                ƒtfails[{{ $i }}] = true
                ƒcancel()
                return
            }
{{ range $sf := $f.SharedFixtures }}
            {{ $sf.LocalVar }} := &{{ $sf.QualifiedType }}{}
            if ƒb, ok := ƒsharedState["{{ $sf.StateKey }}"]; ok {
                if err := json.Unmarshal(ƒb, {{ $sf.LocalVar }}); err != nil {
                    fmt.Fprintf(os.Stderr, "FAIL: unmarshal {{ $sf.FieldName }} state: %v\n", err)
                    ƒtfails[{{ $i }}] = true
                    ƒcancel()
                    return
                }
            }
{{- if $sf.HasHydrate }}
            if err := {{ $sf.LocalVar }}.Hydrate(context.Background()); err != nil {
                fmt.Fprintf(os.Stderr, "FAIL: {{ $sf.FieldName }}.Hydrate: %v\n", err)
                ƒtfails[{{ $i }}] = true
                ƒcancel()
                return
            }
{{- end }}
{{ end }}
{{ end }}
            ƒ_{{ $f.Identifier }} = &{{ $f.QualifiedIdentifier }}{}
{{- range $sf := $f.SharedFixtures }}
            ƒ_{{ $f.Identifier }}.{{ $sf.FieldName }} = {{ $sf.LocalVar }}
{{- end }}
            ƒcfg_{{ $f.Identifier }} = gotest.DefaultFixtureConfig()
{{- if $f.HasConfig }}
            gotest.OverlayFixtureConfig(&ƒcfg_{{ $f.Identifier }}, ƒ_{{ $f.Identifier }}.FixtureConfig())
{{- end }}
            var ƒerr error
            ƒattempts := 1 + ƒcfg_{{ $f.Identifier }}.Retries
            for ƒi := range ƒattempts {
                ctx := ƒctx
                if ƒcfg_{{ $f.Identifier }}.Timeout > 0 {
                    var cancel context.CancelFunc
                    ctx, cancel = context.WithTimeout(ƒctx, ƒcfg_{{ $f.Identifier }}.Timeout)
                    defer cancel()
                }
                ƒerr = ƒ_{{ $f.Identifier }}.BeforeAll(ctx)
                if ƒerr == nil {
                    break
                }
                if ƒctx.Err() != nil {
                    break
                }
                if ƒi < ƒattempts-1 {
                    fmt.Fprintf(os.Stderr, "{{ $f.Identifier }}.BeforeAll attempt %d/%d failed: %v\n", ƒi+1, ƒattempts, ƒerr)
                    if ƒcfg_{{ $f.Identifier }}.RetryDelay > 0 {
                        time.Sleep(ƒcfg_{{ $f.Identifier }}.RetryDelay)
                    }
                }
            }
            if ƒerr != nil {
                fmt.Fprintf(os.Stderr, "FAIL: {{ $f.Identifier }}.BeforeAll failed after %d attempt(s): %v\n", ƒattempts, ƒerr)
                ƒtfails[{{ $i }}] = true
                ƒcancel()
                return
            }
            ƒok_{{ $f.Identifier }} = true
{{ if $f.ChildFixtures }}
            {
                ƒcfails := make([]bool, {{ len $f.ChildFixtures }})
                var ƒcwg sync.WaitGroup
{{ range $j, $cf := $f.ChildFixtures }}
                ƒcwg.Add(1)
                go func() {
                    defer ƒcwg.Done()
                    ƒ_{{ $cf.Identifier }} = &{{ $cf.QualifiedIdentifier }}{
                        {{ $cf.ParentFieldName }}: ƒ_{{ $f.Identifier }},
                    }
                    ƒcfg_{{ $cf.Identifier }} = gotest.DefaultFixtureConfig()
{{- if $cf.HasConfig }}
                    gotest.OverlayFixtureConfig(&ƒcfg_{{ $cf.Identifier }}, ƒ_{{ $cf.Identifier }}.FixtureConfig())
{{- end }}
                    var ƒerr error
                    ƒattempts := 1 + ƒcfg_{{ $cf.Identifier }}.Retries
                    for ƒi := range ƒattempts {
                        ctx := ƒctx
                        if ƒcfg_{{ $cf.Identifier }}.Timeout > 0 {
                            var cancel context.CancelFunc
                            ctx, cancel = context.WithTimeout(ƒctx, ƒcfg_{{ $cf.Identifier }}.Timeout)
                            defer cancel()
                        }
                        ƒerr = ƒ_{{ $cf.Identifier }}.BeforeAll(ctx)
                        if ƒerr == nil {
                            break
                        }
                        if ƒctx.Err() != nil {
                            break
                        }
                        if ƒi < ƒattempts-1 {
                            fmt.Fprintf(os.Stderr, "{{ $cf.Identifier }}.BeforeAll attempt %d/%d failed: %v\n", ƒi+1, ƒattempts, ƒerr)
                            if ƒcfg_{{ $cf.Identifier }}.RetryDelay > 0 {
                                time.Sleep(ƒcfg_{{ $cf.Identifier }}.RetryDelay)
                            }
                        }
                    }
                    if ƒerr != nil {
                        fmt.Fprintf(os.Stderr, "FAIL: {{ $cf.Identifier }}.BeforeAll failed after %d attempt(s): %v\n", ƒattempts, ƒerr)
                        ƒcfails[{{ $j }}] = true
                        ƒcancel()
                        return
                    }
                    ƒok_{{ $cf.Identifier }} = true
                }()
{{ end }}
                ƒcwg.Wait()
                for _, ƒf := range ƒcfails {
                    if ƒf { ƒtfails[{{ $i }}] = true; break }
                }
            }
{{ end }}
        }()
{{ end }}
        ƒswg.Wait()
        for _, ƒf := range ƒtfails {
            if ƒf { return 2 }
        }
    }

    {
        var ƒmaxTreePath time.Duration
        var ƒmaxSuiteSetup time.Duration
{{ range $f := .RootFixtures }}
        {
{{- if $f.ChildFixtures }}
            var ƒmaxChild time.Duration
{{ range $cf := $f.ChildFixtures }}
            if ƒcfg_{{ $cf.Identifier }}.Timeout > ƒmaxChild { ƒmaxChild = ƒcfg_{{ $cf.Identifier }}.Timeout }
{{ end }}
            ƒtreePath := ƒcfg_{{ $f.Identifier }}.Timeout + ƒmaxChild
{{- else }}
            ƒtreePath := ƒcfg_{{ $f.Identifier }}.Timeout
{{- end }}
            if ƒtreePath > ƒmaxTreePath { ƒmaxTreePath = ƒtreePath }
        }
{{ range $ts := $f.ChildSuites }}
        {
            ƒscfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
            gotest.OverlaySuiteConfig(&ƒscfg, (&{{ $ts.Identifier }}{ {{ $ts.FixtureFieldName }}: ƒ_{{ $f.Identifier }} }).SuiteConfig())
{{- end }}
            if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
        }
{{ end }}
{{ range $cf := $f.ChildFixtures }}
{{ range $ts := $cf.ChildSuites }}
        {
            ƒscfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
            gotest.OverlaySuiteConfig(&ƒscfg, (&{{ $ts.Identifier }}{ {{ $ts.FixtureFieldName }}: ƒ_{{ $cf.Identifier }} }).SuiteConfig())
{{- end }}
            if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
        }
{{ end }}
{{ end }}
{{ end }}
        if ƒbudgetFile := os.Getenv("GOTEST_TEARDOWN_BUDGET_FILE"); ƒbudgetFile != "" {
            os.WriteFile(ƒbudgetFile, []byte((ƒmaxTreePath + ƒmaxSuiteSetup + 30*time.Second).String()), 0644)
        }
    }

    ƒcode = m.Run()
    return
}

{{- /* Render fixture-bound suites as top-level functions */ -}}
{{ range $f := .RootFixtures }}
{{ range $ts := $f.ChildSuites }}

func Test{{ $ts.Identifier }}(t *testing.T) {
    s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
        {{ $ts.Identifier }}: {{ $ts.Identifier }}{
            {{ $ts.FixtureFieldName }}: ƒ_{{ $f.Identifier }},
        },
    }
    ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
    gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{- if $ts.IsMethodParallel }}
    wg := &sync.WaitGroup{}
{{- end }}

    ƒsetupT := gotest.NewT(t)
    if ƒcfg.SetupTimeout > 0 {
        ƒsetupT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
    }
    t.Cleanup(func() {
{{- if $ts.IsMethodParallel }}
        wg.Wait()
{{- end }}
        ƒteardownT := gotest.NewT(t)
        if ƒcfg.SetupTimeout > 0 {
            ƒteardownT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
        }
        s.AfterAll(ƒteardownT)
    })
    s.BeforeAll(ƒsetupT)

{{ range $tc := $ts.TestCases }}
    t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $ts.IsMethodParallel }}
        wg.Add(1)
        it.Parallel()
        defer wg.Done()
{{- end }}
        ttt := gotest.NewT(it)
        if ƒcfg.Timeout > 0 {
            ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
        }
{{- if $f.AfterEach }}
        defer func() {
            if err := ƒ_{{ $f.Identifier }}.AfterEach(context.Background()); err != nil {
                it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
            }
        }()
{{- end }}
{{- if $f.BeforeEach }}
        if err := ƒ_{{ $f.Identifier }}.BeforeEach(it.Context()); err != nil {
            it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
        }
{{- end }}
{{- if $ts.HasReturningBeforeEach }}
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

{{- /* Render child fixture's child suites as top-level functions */ -}}
{{ range $cf := $f.ChildFixtures }}
{{ range $ts := $cf.ChildSuites }}

func Test{{ $ts.Identifier }}(t *testing.T) {
    s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
        {{ $ts.Identifier }}: {{ $ts.Identifier }}{
            {{ $ts.FixtureFieldName }}: ƒ_{{ $cf.Identifier }},
        },
    }
    ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
    gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{- if $ts.IsMethodParallel }}
    wg := &sync.WaitGroup{}
{{- end }}

    ƒsetupT := gotest.NewT(t)
    if ƒcfg.SetupTimeout > 0 {
        ƒsetupT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
    }
    t.Cleanup(func() {
{{- if $ts.IsMethodParallel }}
        wg.Wait()
{{- end }}
        ƒteardownT := gotest.NewT(t)
        if ƒcfg.SetupTimeout > 0 {
            ƒteardownT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
        }
        s.AfterAll(ƒteardownT)
    })
    s.BeforeAll(ƒsetupT)

{{ range $tc := $ts.TestCases }}
    t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $ts.IsMethodParallel }}
        wg.Add(1)
        it.Parallel()
        defer wg.Done()
{{- end }}
        ttt := gotest.NewT(it)
        if ƒcfg.Timeout > 0 {
            ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
        }
{{- if $f.AfterEach }}
        defer func() {
            if err := ƒ_{{ $f.Identifier }}.AfterEach(context.Background()); err != nil {
                it.Errorf("{{ $f.Identifier }}.AfterEach failed: %v", err)
            }
        }()
{{- end }}
{{- if $f.BeforeEach }}
        if err := ƒ_{{ $f.Identifier }}.BeforeEach(it.Context()); err != nil {
            it.Fatalf("{{ $f.Identifier }}.BeforeEach failed: %v", err)
        }
{{- end }}
{{- if $cf.AfterEach }}
        defer func() {
            if err := ƒ_{{ $cf.Identifier }}.AfterEach(context.Background()); err != nil {
                it.Errorf("{{ $cf.Identifier }}.AfterEach failed: %v", err)
            }
        }()
{{- end }}
{{- if $cf.BeforeEach }}
        if err := ƒ_{{ $cf.Identifier }}.BeforeEach(it.Context()); err != nil {
            it.Fatalf("{{ $cf.Identifier }}.BeforeEach failed: %v", err)
        }
{{- end }}
{{- if $ts.HasReturningBeforeEach }}
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
{{ end }}
{{ end -}}
