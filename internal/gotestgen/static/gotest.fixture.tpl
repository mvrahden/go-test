{{- /* Shared fixture package-level vars */ -}}
{{ range $sf := .SharedFixtureNodes }}
var ƒ_sf_{{ $sf.Identifier }} = &{{ $sf.QualifiedType }}{}
{{ end }}

{{- /* Package fixture package-level vars */ -}}
{{ range $f := .AllFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedType }}
{{ end }}

{{- /*
  One constructor per DAG node. A hold calls only the constructors of the
  suite's own nodes, so a fixture no suite in the run needs never has its
  config method called.
*/}}
{{ range $sf := .SharedFixtureNodes }}
func ƒ_node_{{ $sf.Identifier }}() *gotestruntime.FixtureNode {
{{- if $sf.HasConfig }}
    ƒcfg := ƒ_sf_{{ $sf.Identifier }}.SharedFixtureConfig()
{{- end }}
    return &gotestruntime.FixtureNode{
        Name: "{{ $sf.Identifier }}",
{{- if $sf.HasConfig }}
        Config: gotestruntime.WithFixtureDefaults(ƒcfg),
        Budget: ƒcfg.Timeout,
{{- else }}
        Config: gotest.DefaultFixtureConfig(),
{{- end }}
        SharedState: &gotestruntime.SharedStateNode{
            StateKey: "{{ $sf.StateKey }}",
            Target: ƒ_sf_{{ $sf.Identifier }},
{{- if $sf.HasHydrate }}
            Hydrate: func(ctx context.Context) error { return ƒ_sf_{{ $sf.Identifier }}.Hydrate(ctx) },
{{- end }}
{{- if $sf.HasDehydrate }}
            Dehydrate: func(ctx context.Context) error { return ƒ_sf_{{ $sf.Identifier }}.Dehydrate(ctx) },
{{- end }}
        },
{{- if $sf.ParentFields }}
        Init: func() {
{{- range $parentID, $fieldName := $sf.ParentFields }}
            ƒ_sf_{{ $sf.Identifier }}.{{ $fieldName }} = ƒ_sf_{{ $parentID }}
{{- end }}
        },
{{- end }}
{{- if $sf.DependsOn }}
        DependsOn: []string{
{{- range $dep := $sf.DependsOn }}
            "{{ $dep }}",
{{- end }}
        },
{{- end }}
    }
}
{{ end }}

{{- range $f := .AllFixtures }}
func ƒ_node_{{ $f.Identifier }}() *gotestruntime.FixtureNode {
{{- if $f.HasConfig }}
    ƒcfg := (&{{ $f.QualifiedType }}{}).FixtureConfig()
{{- end }}
    return &gotestruntime.FixtureNode{
        Name: "{{ $f.Identifier }}",
{{- if $f.HasConfig }}
        Config: gotestruntime.WithFixtureDefaults(ƒcfg),
        Budget: ƒcfg.Timeout,
{{- else }}
        Config: gotest.DefaultFixtureConfig(),
{{- end }}
        Init: func() {
{{- if or $f.ParentFieldNames $f.SharedFixtures }}
            ƒ_{{ $f.Identifier }} = &{{ $f.QualifiedType }}{
{{- range $parentID, $fieldName := $f.ParentFieldNames }}
                {{ $fieldName }}: ƒ_{{ $parentID }},
{{- end }}
{{- range $sf := $f.SharedFixtures }}
                {{ $sf.FieldName }}: ƒ_sf_{{ $sf.Identifier }},
{{- end }}
            }
{{- else }}
            ƒ_{{ $f.Identifier }} = &{{ $f.QualifiedType }}{}
{{- end }}
        },
        BeforeAll: func(ctx context.Context) error {
            return ƒ_{{ $f.Identifier }}.BeforeAll(ctx)
        },
{{- if $f.AfterAll }}
        AfterAll: func(ctx context.Context) error {
            return ƒ_{{ $f.Identifier }}.AfterAll(ctx)
        },
{{- end }}
{{- if $f.DependsOn }}
        DependsOn: []string{
{{- range $dep := $f.DependsOn }}
            "{{ $dep }}",
{{- end }}
        },
{{- end }}
    }
}
{{ end }}

{{- /*
  Built per hold, not at package init: a panicking config method fails the
  holding test instead of the binary. Every hold starts from nil fixtures, so
  nothing reads one an earlier hold tore down.
*/}}
func ƒ_holdFixtures(tb testing.TB, setupTimeout func() time.Duration, nodes ...func() *gotestruntime.FixtureNode) {
    gotestruntime.HoldFixtures(tb, func() gotestruntime.MainConfig {
{{- range $f := .AllFixtures }}
        ƒ_{{ $f.Identifier }} = nil
{{- end }}
        ƒmain := gotestruntime.MainConfig{MaxSuiteSetupTimeout: setupTimeout()}
        for _, node := range nodes {
            ƒmain.Fixtures = append(ƒmain.Fixtures, node())
        }
        return ƒmain
    })
}
