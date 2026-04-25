{{- /* Declare compile time assertion of enum set constants */ -}}
{{ range $i, $ts := .Spec.EffectiveTestSuites }}

type ƒƒ_GOTEST_{{ $ts.Identifier }} struct {
  {{ $ts.Identifier }}
}

func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeAll(it *gotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterAll(it *gotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach(it) {{ end }}}

func Test{{ $ts.Identifier }}(t *testing.T) {
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}
{{- if (hasSuffix $ts.FullIdentifier "TestSuiteParallel") }}
  t.Parallel()
{{- end }}

{{ if $ts.TestCases -}}
  newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
    return func(tt *gotest.T) {
      t := tt.T()
      t.Run(desc, func(it *testing.T) {
        ttt := gotest.NewT(it)
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        testFn(ttt)
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
        defer s.AfterEach(ttt)
        s.BeforeEach(ttt)
        testFn(ttt)
      })
    }}
  wg := &sync.WaitGroup{}
{{- end }}

  testCases := []gotest.TestCase{
{{- range $tc := $ts.TestCases }}
  {{- if not $tc.IsParallel }}
    newTestCase("{{ $tc.Identifier }}", s.{{ $tc.Identifier }}),
  {{- else }}
    newParallelTestCase("{{ $tc.Identifier }}", wg, s.{{ $tc.Identifier }}),
  {{- end }}
{{- end }}
  }

  tt := gotest.NewT(t)
  t.Cleanup(func() {
{{- /*
    Require this call before BeforeAll, as Cleanup calls are LIFO and could be registered in BeforeAll by user.
    */ -}}
{{- if $ts.HasParallelTestCases }}
    wg.Wait()
{{- end }}
    s.AfterAll(tt)
  })
  s.BeforeAll(tt)
  for _, tc := range testCases {
    tc(tt)
  }
}
{{- end }}

{{ range $ts := .Spec.SkippedTestSuites }}
func Test{{ $ts.Identifier }}(t *testing.T) {
  t.Skipf("test suite was excluded by user")
}

{{ end -}}
