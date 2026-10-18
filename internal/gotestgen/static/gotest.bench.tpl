{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- if $ts.Benchmarks }}
{{- $fx := index $.Fixtures $ts.Identifier }}
func Benchmark{{ $ts.Identifier }}(b *ƒtesting.B) {
{{- template "suiteFrame" (dict "Suite" $ts "Fixtures" $fx "TB" "b" "Assign" "") }}
{{ range $bm := $ts.Benchmarks }}
  b.Run("{{ $bm.Identifier }}", func(b *ƒtesting.B) {
    defer ƒgotestruntime.ReleaseOnPanic(b)
    ƒgotestruntime.SkipIfStopped(b)
    b.StopTimer()
    ƒeachT := ƒgotest.NewTFromTB(b)
    defer ƒgotestruntime.AfterEach(func() {
      b.StopTimer()
      s.AfterEach(ƒeachT)
    })
    s.BeforeEach(ƒeachT)
    b.StartTimer()
    b.ResetTimer()
    ƒgotestruntime.RunBody(b, func() { s.{{ $bm.Identifier }}({{ if $bm.UsesStdlibT }}b{{ else }}ƒgotest.NewB(b){{ end }}) })
  })
{{ end }}
}
{{- end }}
{{- end }}
