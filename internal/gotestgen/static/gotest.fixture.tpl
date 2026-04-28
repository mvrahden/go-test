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
{{- /* Resolve shared fixture embeddings from env */ -}}
{{ range $sf := $f.SharedFixtures }}
    {{ $sf.LocalVar }} := &{{ $sf.QualifiedType }}{}
{{- range $field, $env := $sf.EnvTags }}
    {{ $sf.LocalVar }}.{{ $field }} = os.Getenv("{{ $env }}")
    if {{ $sf.LocalVar }}.{{ $field }} == "" {
        t.Fatal("{{ $env }} not set — run via testsuite CLI")
    }
{{- end }}
{{ end }}

    fixture := &{{ $f.Identifier }}{}
{{- range $sf := $f.SharedFixtures }}
    fixture.{{ $sf.FieldName }} = {{ $sf.LocalVar }}
{{- end }}
    t.Cleanup(func() {
{{- if $f.AfterAll }}
        if err := fixture.AfterAll(context.Background()); err != nil {
            t.Errorf("{{ $f.Identifier }}.AfterAll failed: %v", err)
        }
{{- end }}
    })
    if err := fixture.BeforeAll(t.Context()); err != nil {
        t.Fatalf("{{ $f.Identifier }}.BeforeAll failed: %v", err)
    }

{{- /* Render child suites */ -}}
{{ range $ts := $f.ChildSuites }}
    t.Run("{{ $ts.Identifier }}", func(t *testing.T) {
        s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
            {{ $ts.Identifier }}: {{ $ts.Identifier }}{
                {{ $f.Identifier }}: fixture,
            },
        }
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
        t.Parallel()
{{- end }}

{{ if $ts.TestCases -}}
        newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
            return func(tt *gotest.T) {
                t := tt.T()
                t.Run(desc, func(it *testing.T) {
                    ttt := gotest.NewT(it)
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
        }
    })
{{ end }}

{{- /* Render child fixtures (Level 2 nesting) */ -}}
{{ range $cf := $f.ChildFixtures }}
    t.Run("{{ $cf.Identifier }}", func(t *testing.T) {
        child := &{{ $cf.Identifier }}{
            {{ $f.Identifier }}: fixture,
        }
        t.Cleanup(func() {
{{- if $cf.AfterAll }}
            if err := child.AfterAll(context.Background()); err != nil {
                t.Errorf("{{ $cf.Identifier }}.AfterAll failed: %v", err)
            }
{{- end }}
        })
        if err := child.BeforeAll(t.Context()); err != nil {
            t.Fatalf("{{ $cf.Identifier }}.BeforeAll failed: %v", err)
        }

{{- range $ts := $cf.ChildSuites }}
        t.Run("{{ $ts.Identifier }}", func(t *testing.T) {
            s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
                {{ $ts.Identifier }}: {{ $ts.Identifier }}{
                    {{ $cf.Identifier }}: child,
                },
            }
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
            t.Parallel()
{{- end }}

{{ if $ts.TestCases -}}
            newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
                return func(tt *gotest.T) {
                    t := tt.T()
                    t.Run(desc, func(it *testing.T) {
                        ttt := gotest.NewT(it)
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
            }
        })
{{- end }}
    })
{{ end }}
}
{{ end -}}
