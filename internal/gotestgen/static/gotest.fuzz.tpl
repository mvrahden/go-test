{{- with $.FuzzCodecSource }}
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
  Harvested seeds go to *testing.F directly, BEFORE gotest.NewF exists, so
  they are never codec-encoded. Safe today: the harvester only matches basic
  literals whose type is identical to the callback param, which no struct
  param can satisfy, so struct targets harvest nothing. If harvesting is ever
  widened to struct composite literals, these lines must move below the
  gotest.NewF call and go through *gotest.F — testing.F.Add panics on a
  struct with "unsupported type to Add".
*/}}
{{- range index $.HarvestedSeeds $funcName }}
  f.Add({{ . }})
{{- end }}
  s.{{ $fz.Identifier }}(gotest.NewF(f, s.BeforeEach, s.AfterEach{{ range $c := $.FuzzCodecs }}, gotest.Codec[{{ $c.TypeRef }}]{Decode: {{ $c.DecodeFunc }}, Encode: {{ $c.EncodeFunc }}}{{ end }}))
}
{{ end }}
{{- end }}
