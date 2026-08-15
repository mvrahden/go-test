{{- with $.FuzzFanSource }}
{{ . }}
{{- end }}
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
{{- $funcName := printf "Fuzz%s_%s" $ts.Identifier $fz.Identifier }}
{{- /*
  Harvested seeds go through *gotest.F, never *testing.F directly: F.Add
  buffers them and gotest.Fuzz explodes them through the target's own fan
  at flush time, so a harvested int seed on a fanned numeric position is
  encoded exactly like a hand-written one. They are added before the user's
  method runs, so they precede the method's own f.Add seeds in replay order.
*/}}
  ƒf := gotest.NewF(f, s.BeforeEach, s.AfterEach{{ range $c := $.FuzzFans }}, {{ $c.Expr }}{{ end }})
{{- range index $.HarvestedSeeds $funcName }}
  ƒf.Add({{ . }})
{{- end }}
  s.{{ $fz.Identifier }}(ƒf)
}
{{ end }}
{{- end }}
