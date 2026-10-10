{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- $fx := index $.Fixtures $ts.Identifier }}

{{ template "suiteWrapper" $ts }}
{{ if $ts.TestCases }}
func Test{{ $ts.Identifier }}(t *testing.T) {
{{- template "suiteInstance" (dict "Suite" $ts "Fixtures" $fx "TB" "t") }}
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

{{ template "suiteMethods" (dict "Suite" $ts "FixtureOrder" $fx.Order) }}
}
{{- else if $ts.DeclaresTests }}
{{ template "excludedSuite" $ts }}
{{- end }}
{{- end }}

{{ range $ts := .Spec.SkippedTestSuites }}
{{ template "excludedSuite" $ts }}

{{ end -}}
