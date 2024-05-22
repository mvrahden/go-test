{{- /* Declare compile time assertion of enum set constants */ -}}
{{ range $i, $ts := .Spec.EffectiveTestSuites }}

type ƒƒ_GOTEST_{{ if not $ts.IsTestPackageSuite }}{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }} struct {
  {{ $ts.FullIdentifier }}
}

func (ts *ƒƒ_GOTEST_{{ if not $ts.IsTestPackageSuite }}{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}) BeforeAll(it *gotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ if not $ts.IsTestPackageSuite }}{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}) AfterAll(it *gotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ if not $ts.IsTestPackageSuite }}{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}) BeforeEach(it *gotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach(it) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ if not $ts.IsTestPackageSuite }}{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}) AfterEach(it *gotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach(it) {{ end }}}

func Test{{ if not $ts.IsTestPackageSuite }}_{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}(t *testing.T) {
  {{ if (eq $i 0) -}}
  t.Cleanup(func() {
    err := os.Remove("./gotest_gensuite_test.go")
    if err != nil {
      t.Logf("failed removing test suite: %s", err.Error())
    }
  })
  {{ end -}}
  s := &ƒƒ_GOTEST_{{- if not $ts.IsTestPackageSuite -}}{{ $ts.PackageName }}_{{- end -}}{{ $ts.Identifier }}{}
{{- if (hasSuffix $ts.FullIdentifier "ParallelTestSuite") }}
  t.Parallel()
{{- end }}

{{ if $ts.TestCases -}}
  newTestCase := func(desc string, testFn gotest.TestCase) gotest.TestCase {
    return func(tt *gotest.T) {
      t := tt.T()
      t.Run(desc, func(it *testing.T) {
        ttt := gotest.NewT(it)
        s.BeforeEach(ttt)
        testFn(ttt)
        s.AfterEach(ttt)
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
        ttt := gotest.NewT(it)
        s.BeforeEach(ttt)
        testFn(ttt)
        s.AfterEach(ttt)
        wg.Done()
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
func Test{{ if not $ts.IsTestPackageSuite }}_{{ $ts.PackageName }}_{{ end -}}{{ $ts.Identifier }}(t *testing.T) {
  t.Skipf("test suite was excluded by user")
}

{{ end -}}
