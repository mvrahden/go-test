{{- with $.FuzzFanSource }}
{{ . }}
{{- end }}
{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- range $fz := $ts.Fuzzers }}
func Fuzz{{ $ts.Identifier }}_{{ $fz.Identifier }}(f *testing.F) {
{{- template "suiteInstance" (dict "Suite" $ts "Fixtures" (index $.Fixtures $ts.Identifier) "TB" "f") }}
  gotestruntime.OpenSuite(f, gotestruntime.Suite{
{{- if $ts.HasGuard }}
    Guard: s.{{ $ts.Identifier }}.SuiteGuard,
{{- end }}
{{- if $ts.HasConfig }}
    Config: s.{{ $ts.Identifier }}.SuiteConfig,
{{- end }}
    BeforeAll: s.BeforeAll,
    AfterAll: s.AfterAll,
  })
{{- $funcName := printf "Fuzz%s_%s" $ts.Identifier $fz.Identifier }}
{{- /*
  Harvested seeds go through *gotest.F, never *testing.F directly: F.Add
  buffers them and f.Fuzz explodes them through the generated target at
  flush time, so a harvested int seed on a fanned numeric position is
  encoded exactly like a hand-written one. They are added before the user's
  method runs, so they precede the method's own f.Add seeds in replay order.
*/}}
  ƒf := gotest.NewF(f, s.BeforeEach, s.AfterEach, {{ with index $.FuzzTargets $funcName }}{{ . }}{{ else }}nil{{ end }})
{{- range index $.HarvestedSeeds $funcName }}
  ƒf.Add({{ . }})
{{- end }}
  s.{{ $fz.Identifier }}(ƒf)
}
{{ end }}
{{- end }}
