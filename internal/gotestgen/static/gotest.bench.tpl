{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- if $ts.Benchmarks }}
func Benchmark{{ $ts.Identifier }}(b *testing.B) {
{{- $fx := index $.SuiteFixtures $ts.Identifier }}
{{- $sfRefs := index $.SuiteSharedFixtures $ts.Identifier }}
{{- if or $fx $sfRefs }}
  ƒ_setupFixtures(b)
{{- end }}
{{- if $fx }}
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{
    {{ $ts.Identifier }}: {{ $ts.Identifier }}{
{{- range $id, $field := $fx.FixtureFields }}
      {{ $field }}: ƒ_{{ $id }},
{{- end }}
    },
  }
{{- else }}
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}
{{- end }}
{{- if $ts.HasGuard }}
  if ƒreason := s.{{ $ts.Identifier }}.SuiteGuard(); ƒreason != "" {
    b.Skipf("suite guard: %s", ƒreason)
    return
  }
{{- end }}
{{- if not $fx }}
{{- range $sf := $sfRefs }}
  s.{{ $sf.FieldName }} = ƒ_sf_{{ $sf.Identifier }}
{{- end }}
{{- end }}
  ƒlifecycleT := gotest.NewTFromTB(b)
  b.Cleanup(func() { s.AfterAll(gotest.NewTFromTB(b)) })
  s.BeforeAll(ƒlifecycleT)
{{ range $bm := $ts.Benchmarks }}
  b.Run("{{ $bm.Identifier }}", func(b *testing.B) {
    b.StopTimer()
    ƒeachT := gotest.NewTFromTB(b)
    s.BeforeEach(ƒeachT)
    b.StartTimer()
    b.ResetTimer()
    s.{{ $bm.Identifier }}({{ if $bm.UsesStdlibT }}b{{ else }}gotest.NewB(b){{ end }})
    b.StopTimer()
    s.AfterEach(ƒeachT)
  })
{{ end }}
}
{{- end }}
{{- end }}
