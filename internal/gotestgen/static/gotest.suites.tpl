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
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}
{{- if $ts.HasGuard }}
  if ƒreason := s.{{ $ts.Identifier }}.SuiteGuard(); ƒreason != "" {
    t.Skipf("suite guard: %s", ƒreason)
    return
  }
{{- end }}
{{- $sfRefs := index $.SuiteSharedFixtures $ts.Identifier }}
{{- if $sfRefs }}
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
{{ range $sf := $sfRefs }}
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
  s.{{ $sf.FieldName }} = {{ $sf.LocalVar }}
{{ end }}
{{- end }}
  ƒcfg := gotest.DefaultSuiteConfig()
{{- if $ts.HasConfig }}
  gotest.OverlaySuiteConfig(&ƒcfg, s.{{ $ts.Identifier }}.SuiteConfig())
{{- end }}

{{- if $ts.IsMethodParallel }}
  wg := &sync.WaitGroup{}
{{- end }}

  tt := gotest.NewT(t)
  t.Cleanup(func() {
{{- if $ts.IsMethodParallel }}
    wg.Wait()
{{- end }}
    s.AfterAll(tt)
  })
  s.BeforeAll(tt)

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
