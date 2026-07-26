{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- range $fz := $ts.Fuzzers }}
func Fuzz{{ $ts.Identifier }}_{{ $fz.Identifier }}(f *testing.F) {
{{- $fx := index $.SuiteFixtures $ts.Identifier }}
{{- $sfRefs := index $.SuiteSharedFixtures $ts.Identifier }}
{{- if or $fx $sfRefs }}
  ƒ_setupFixtures(f)
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
    f.Skipf("suite guard: %s", ƒreason)
    return
  }
{{- end }}
{{- if not $fx }}
{{- range $sf := $sfRefs }}
  s.{{ $sf.FieldName }} = ƒ_sf_{{ $sf.Identifier }}
{{- end }}
{{- end }}
  ƒlifecycleT := gotest.NewTFromTB(f)
  f.Cleanup(func() { s.AfterAll(gotest.NewTFromTB(f)) })
  s.BeforeAll(ƒlifecycleT)
  s.{{ $fz.Identifier }}(gotest.NewF(f, s.BeforeEach, s.AfterEach))
}
{{ end }}
{{- end }}
