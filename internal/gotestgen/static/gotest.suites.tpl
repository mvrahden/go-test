{{ range $i, $ts := .Spec.EffectiveTestSuites }}

{{ template "suiteWrapper" $ts }}
{{ if $ts.TestCases }}
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
  ƒcfg, ƒbudget := gotestruntime.OpenSuite(t, gotestruntime.Suite{
{{- if $ts.HasGuard }}
    Guard: s.{{ $ts.Identifier }}.SuiteGuard,
{{- end }}
{{- if $ts.HasConfig }}
    Config: s.{{ $ts.Identifier }}.SuiteConfig,
{{- end }}
    BeforeAll: s.BeforeAll,
    AfterAll: s.AfterAll,
  })
{{- if $ts.IsMethodParallel }}
  ƒfailed := &atomic.Bool{}
{{- end }}

{{ template "suiteMethods" (dict "Suite" $ts) }}
}
{{- else if $ts.DeclaresTests }}
{{ template "excludedSuite" $ts }}
{{- end }}
{{- end }}

{{ range $ts := .Spec.SkippedTestSuites }}
{{ template "excludedSuite" $ts }}

{{ end -}}
