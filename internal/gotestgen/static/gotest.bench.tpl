{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- if $ts.Benchmarks }}
{{- $fx := index $.Fixtures $ts.Identifier }}
func Benchmark{{ $ts.Identifier }}(b *testing.B) {
{{- template "suiteFrame" (dict "Suite" $ts "Fixtures" $fx "TB" "b" "Assign" "") }}
{{ range $bm := $ts.Benchmarks }}
  b.Run("{{ $bm.Identifier }}", func(b *testing.B) {
{{- if $fx.Nodes }}
    defer gotestruntime.ReleaseOnPanic(b)
{{- end }}
    b.StopTimer()
    ƒeachT := gotest.NewTFromTB(b)
    defer func() {
      b.StopTimer()
      s.AfterEach(ƒeachT)
    }()
    s.BeforeEach(ƒeachT)
    b.StartTimer()
    b.ResetTimer()
    s.{{ $bm.Identifier }}({{ if $bm.UsesStdlibT }}b{{ else }}gotest.NewB(b){{ end }})
  })
{{ end }}
}
{{- end }}
{{- end }}
