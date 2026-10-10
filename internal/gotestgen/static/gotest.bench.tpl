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
{{- if not $fx }}
{{- range $sf := $sfRefs }}
  s.{{ $sf.FieldName }} = ƒ_sf_{{ $sf.Identifier }}
{{- end }}
{{- end }}
  gotestruntime.OpenSuite(b, gotestruntime.Suite{
{{- if $ts.HasGuard }}
    Guard: s.{{ $ts.Identifier }}.SuiteGuard,
{{- end }}
{{- if $ts.HasConfig }}
    Config: s.{{ $ts.Identifier }}.SuiteConfig,
{{- end }}
    BeforeAll: s.BeforeAll,
    AfterAll: s.AfterAll,
  })
{{ range $bm := $ts.Benchmarks }}
  b.Run("{{ $bm.Identifier }}", func(b *testing.B) {
{{- if or $fx $sfRefs }}
    defer gotestruntime.ReleaseOnPanic(b)
{{- end }}
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
