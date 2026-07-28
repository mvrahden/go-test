{{ range $i, $ts := .Spec.EffectiveTestSuites }}

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

func Test{{ $ts.Identifier }}(t *testing.T) {
{{- $sfRefs := index $.SuiteSharedFixtures $ts.Identifier }}
{{- if $sfRefs }}
  ƒ_setupFixtures(t)
{{- end }}
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}
{{- if $ts.HasGuard }}
  if ƒreason := s.{{ $ts.Identifier }}.SuiteGuard(); ƒreason != "" {
    t.Skipf("suite guard: %s", ƒreason)
    return
  }
{{- end }}
{{- if $sfRefs }}
{{ range $sf := $sfRefs }}
  s.{{ $sf.FieldName }} = ƒ_sf_{{ $sf.Identifier }}
{{- end }}
{{- end }}
  ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
  gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

  ƒsetupT := gotest.NewT(t)
  if ƒcfg.SetupTimeout > 0 {
    ƒsetupT = gotest.NewTWithDeadline(t, ƒcfg.SetupTimeout)
  }
  // testing runs this cleanup only after every subtest started via t.Run has
  // finished, parallel ones included. It must never wait on those subtests
  // itself: on panic the testing package runs ancestor cleanups from the
  // panicking goroutine, so such a wait deadlocks against the panic unwind.
  t.Cleanup(func() {
    s.AfterAll(gotest.NewTeardownT(t, ƒcfg.SetupTimeout))
  })
  s.BeforeAll(ƒsetupT)

{{ range $tc := $ts.TestCases }}
  t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $ts.IsMethodParallel }}
    it.Parallel()
{{- end }}
    ttt := gotest.NewT(it)
    if ƒcfg.Timeout > 0 {
        ttt = gotest.NewTWithDeadline(it, ƒcfg.Timeout)
    }
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
{{- end }}

{{ range $ts := .Spec.SkippedTestSuites }}
func Test{{ $ts.Identifier }}(t *testing.T) {
  t.Skipf("test suite was excluded by user")
}

{{ end -}}
