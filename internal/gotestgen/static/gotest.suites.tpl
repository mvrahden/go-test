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
{{- if $sfRefs }}
{{ range $sf := $sfRefs }}
  s.{{ $sf.FieldName }} = ƒ_sf_{{ $sf.Identifier }}
{{- end }}
{{- end }}
{{- if $ts.HasGuard }}
  if ƒreason := s.{{ $ts.Identifier }}.SuiteGuard(); ƒreason != "" {
    t.Skipf("suite guard: %s", ƒreason)
    return
  }
{{- end }}
{{- if $ts.HasConfig }}
  ƒcfg := s.{{ $ts.Identifier }}.SuiteConfig()
  ƒbudget := ƒcfg
{{- else }}
  ƒcfg := gotest.DefaultSuiteConfig()
  ƒbudget := gotest.SuiteConfig{}
{{- end }}
{{- if and $ts.IsMethodParallel $ts.TestCases }}
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

{{ range $tc := $ts.TestCases }}
  t.Run("{{ $tc.Identifier }}", func(it *testing.T) {
{{- if $ts.IsMethodParallel }}
    it.Parallel()
    if ƒcfg.FailFast && ƒfailed.Load() {
      it.Skip("FailFast: earlier test failed")
    }
    defer func() { if it.Failed() { ƒfailed.Store(true) } }()
{{- end }}
    ttt := gotestruntime.TestT(it, ƒcfg.Timeout)
{{- if $ts.HasReturningBeforeEach }}
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
{{- if not $ts.IsMethodParallel }}
  if ƒcfg.FailFast && t.Failed() {
    return
  }
{{- end }}
{{ end }}
}
{{- end }}

{{ range $ts := .Spec.SkippedTestSuites }}
func Test{{ $ts.Identifier }}(t *testing.T) {
  t.Skipf("test suite was excluded by user")
}

{{ end -}}
