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

{{- /* Package-level fixture variables */ -}}
{{ range $f := .RootFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedIdentifier }}
{{ range $cf := $f.ChildFixtures }}
var ƒ_{{ $cf.Identifier }} *{{ $cf.QualifiedIdentifier }}
{{ end }}
{{ end }}

func TestMain(m *testing.M) { os.Exit(ƒƒ_GOTEST_main(m)) }

func ƒƒ_GOTEST_main(m *testing.M) (ƒcode int) {
{{ range $f := .RootFixtures }}
{
{{- /* Resolve shared fixture embeddings from JSON state */ -}}
{{ if $f.SharedFixtures }}
    ƒsharedFile := os.Getenv("GOTEST_SHARED_STATE_FILE")
    if ƒsharedFile == "" {
        fmt.Fprintln(os.Stderr, "FAIL: GOTEST_SHARED_STATE_FILE not set — run via gotest CLI")
        return 2
    }
    ƒsharedRaw, ƒsharedErr := os.ReadFile(ƒsharedFile)
    if ƒsharedErr != nil {
        fmt.Fprintf(os.Stderr, "FAIL: read shared state file: %v\n", ƒsharedErr)
        return 2
    }
    ƒsharedState := map[string]json.RawMessage{}
    if err := json.Unmarshal(ƒsharedRaw, &ƒsharedState); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL: unmarshal shared state: %v\n", err)
        return 2
    }
{{ range $sf := $f.SharedFixtures }}
    {{ $sf.LocalVar }} := &{{ $sf.QualifiedType }}{}
    if ƒb, ok := ƒsharedState["{{ $sf.StateKey }}"]; ok {
        if err := json.Unmarshal(ƒb, {{ $sf.LocalVar }}); err != nil {
            fmt.Fprintf(os.Stderr, "FAIL: unmarshal {{ $sf.FieldName }} state: %v\n", err)
            return 2
        }
    }
{{- if $sf.HasHydrate }}
    if err := {{ $sf.LocalVar }}.Hydrate(context.Background()); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL: {{ $sf.FieldName }}.Hydrate: %v\n", err)
        return 2
    }
{{- end }}
{{- if $sf.HasDehydrate }}
    defer {{ $sf.LocalVar }}.Dehydrate(context.Background())
{{- end }}
{{ end }}
{{ end }}

    ƒ_{{ $f.Identifier }} = &{{ $f.QualifiedIdentifier }}{}
{{- range $sf := $f.SharedFixtures }}
    ƒ_{{ $f.Identifier }}.{{ $sf.FieldName }} = {{ $sf.LocalVar }}
{{- end }}
    ƒcfg := gotest.DefaultFixtureConfig()
{{- if $f.HasConfig }}
    gotest.OverlayFixtureConfig(&ƒcfg, ƒ_{{ $f.Identifier }}.FixtureConfig())
{{- end }}
    var ƒerr error
    ƒattempts := 1 + ƒcfg.Retries
    for ƒi := range ƒattempts {
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        ƒerr = ƒ_{{ $f.Identifier }}.BeforeAll(ctx)
        if ƒerr == nil {
            break
        }
        if ƒi < ƒattempts-1 {
            fmt.Fprintf(os.Stderr, "{{ $f.Identifier }}.BeforeAll attempt %d/%d failed: %v\n", ƒi+1, ƒattempts, ƒerr)
            if ƒcfg.RetryDelay > 0 {
                time.Sleep(ƒcfg.RetryDelay)
            }
        }
    }
    if ƒerr != nil {
        fmt.Fprintf(os.Stderr, "FAIL: {{ $f.Identifier }}.BeforeAll failed after %d attempt(s): %v\n", ƒattempts, ƒerr)
        return 2
    }
{{- if $f.AfterAll }}
    defer func() {
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        if err := ƒ_{{ $f.Identifier }}.AfterAll(ctx); err != nil {
            fmt.Fprintf(os.Stderr, "{{ $f.Identifier }}.AfterAll failed: %v\n", err)
            if ƒcode == 0 { ƒcode = 1 }
        }
    }()
{{- end }}
}

{{- /* Setup child fixtures */ -}}
{{ range $cf := $f.ChildFixtures }}
{
    ƒ_{{ $cf.Identifier }} = &{{ $cf.QualifiedIdentifier }}{
        {{ $cf.ParentFieldName }}: ƒ_{{ $f.Identifier }},
    }
    ƒcfg := gotest.DefaultFixtureConfig()
{{- if $cf.HasConfig }}
    gotest.OverlayFixtureConfig(&ƒcfg, ƒ_{{ $cf.Identifier }}.FixtureConfig())
{{- end }}
    var ƒerr error
    ƒattempts := 1 + ƒcfg.Retries
    for ƒi := range ƒattempts {
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        ƒerr = ƒ_{{ $cf.Identifier }}.BeforeAll(ctx)
        if ƒerr == nil {
            break
        }
        if ƒi < ƒattempts-1 {
            fmt.Fprintf(os.Stderr, "{{ $cf.Identifier }}.BeforeAll attempt %d/%d failed: %v\n", ƒi+1, ƒattempts, ƒerr)
            if ƒcfg.RetryDelay > 0 {
                time.Sleep(ƒcfg.RetryDelay)
            }
        }
    }
    if ƒerr != nil {
        fmt.Fprintf(os.Stderr, "FAIL: {{ $cf.Identifier }}.BeforeAll failed after %d attempt(s): %v\n", ƒattempts, ƒerr)
        return 2
    }
{{- if $cf.AfterAll }}
    defer func() {
        ctx := context.Background()
        if ƒcfg.Timeout > 0 {
            var cancel context.CancelFunc
            ctx, cancel = context.WithTimeout(ctx, ƒcfg.Timeout)
            defer cancel()
        }
        if err := ƒ_{{ $cf.Identifier }}.AfterAll(ctx); err != nil {
            fmt.Fprintf(os.Stderr, "{{ $cf.Identifier }}.AfterAll failed: %v\n", err)
            if ƒcode == 0 { ƒcode = 1 }
        }
    }()
{{- end }}
}
{{ end }}
{{ end }}

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
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
    t.Parallel()
{{- end }}
    ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
    gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{ if $ts.TestCases -}}
    newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
        return func(tt *gotest.T) {
            t := tt.T()
            t.Run(desc, func(it *testing.T) {
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
        if ƒcfg.FailFast && t.Failed() {
            break
        }
    }
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
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
    t.Parallel()
{{- end }}
    ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
    gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{ if $ts.TestCases -}}
    newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
        return func(tt *gotest.T) {
            t := tt.T()
            t.Run(desc, func(it *testing.T) {
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
        if ƒcfg.FailFast && t.Failed() {
            break
        }
    }
}
{{ end }}
{{ end }}
{{ end -}}
