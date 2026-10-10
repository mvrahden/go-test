{{ range $i, $ts := .Spec.EffectiveTestSuites }}
{{- $fx := index $.Fixtures $ts.Identifier }}

{{ template "suiteWrapper" $ts }}
{{ if $ts.TestCases }}
func Test{{ $ts.Identifier }}(t *testing.T) {
{{- template "suiteFrame" (dict "Suite" $ts "Fixtures" $fx "TB" "t" "Assign" "ƒcfg, ƒbudget := ") }}
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
