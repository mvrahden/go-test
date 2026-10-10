{{- /* Declare wrapper structs for all fixture-bound suites at file scope */ -}}
{{ range $ts := .FixtureBoundSuites }}

{{ template "suiteWrapper" $ts }}
{{- end }}

{{- /* Shared fixture package-level vars */ -}}
{{ range $sf := .SharedFixtureNodes }}
var ƒ_sf_{{ $sf.Identifier }} = &{{ $sf.QualifiedType }}{}
{{- if $sf.HasConfig }}
var ƒcfg_sf_{{ $sf.Identifier }} gotest.FixtureConfig
{{- end }}
{{ end }}

{{- /* Package fixture package-level vars */ -}}
{{ range $f := .AllFixtures }}
var ƒ_{{ $f.Identifier }} *{{ $f.QualifiedType }}
{{- if $f.HasConfig }}
var ƒcfg_{{ $f.Identifier }} gotest.FixtureConfig
{{- end }}
{{ end }}

func ƒ_setupFixtures(t testing.TB) {
    gotestruntime.HoldFixtures(t, func() gotestruntime.MainConfig {
{{- /*
  Built per hold, not at package init: a panicking config method fails the
  holding test instead of the binary. Every hold starts from nil fixtures, so
  nothing reads one an earlier hold tore down.
*/}}
{{- range $f := .AllFixtures }}
        ƒ_{{ $f.Identifier }} = nil
{{- end }}
{{- range $sf := .SharedFixtureNodes }}
{{- if $sf.HasConfig }}
        ƒcfg_sf_{{ $sf.Identifier }} = ƒ_sf_{{ $sf.Identifier }}.SharedFixtureConfig()
{{- end }}
{{- end }}
{{- range $f := .AllFixtures }}
{{- if $f.HasConfig }}
        ƒcfg_{{ $f.Identifier }} = (&{{ $f.QualifiedType }}{}).FixtureConfig()
{{- end }}
{{- end }}
        var ƒmaxSuiteSetup time.Duration
{{ range $fs := .FlatSuites }}
        {
{{- if $fs.Suite.HasConfig }}
            ƒscfg := gotestruntime.WithSuiteDefaults((&{{ $fs.Suite.Identifier }}{
{{- range $id, $field := $fs.FixtureFields }}
                {{ $field }}: ƒ_{{ $id }},
{{- end }}
            }).SuiteConfig())
{{- else }}
            ƒscfg := gotest.DefaultSuiteConfig()
{{- end }}
            if ƒscfg.SetupTimeout > ƒmaxSuiteSetup { ƒmaxSuiteSetup = ƒscfg.SetupTimeout }
        }
{{ end }}

        return gotestruntime.MainConfig{
            Fixtures: []*gotestruntime.FixtureNode{
{{- range $sf := .SharedFixtureNodes }}
                {
                    Name: "{{ $sf.Identifier }}",
{{- if $sf.HasConfig }}
                    Config: gotestruntime.WithFixtureDefaults(ƒcfg_sf_{{ $sf.Identifier }}),
                    Budget: ƒcfg_sf_{{ $sf.Identifier }}.Timeout,
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
                },
{{- end }}
{{- range $f := .AllFixtures }}
{{ template "fixtureNode" $f }}
{{- end }}
            },
            MaxSuiteSetupTimeout: ƒmaxSuiteSetup,
        }
    })
}

{{- /* Render fixture-bound suites as top-level Test functions */ -}}
{{ range $fs := .FlatSuites }}

func Test{{ $fs.Suite.Identifier }}(t *testing.T) {
    ƒ_setupFixtures(t)

    s := &ƒƒ_GOTEST_{{ $fs.Suite.Identifier }}{
        {{ $fs.Suite.Identifier }}: {{ $fs.Suite.Identifier }}{
{{- range $id, $field := $fs.FixtureFields }}
            {{ $field }}: ƒ_{{ $id }},
{{- end }}
        },
    }
    {{ if $fs.Suite.TestCases }}ƒcfg, ƒbudget := {{ end }}gotestruntime.OpenSuite(t, gotestruntime.Suite{
{{- if $fs.Suite.HasGuard }}
      Guard: s.{{ $fs.Suite.Identifier }}.SuiteGuard,
{{- end }}
{{- if $fs.Suite.HasConfig }}
      Config: s.{{ $fs.Suite.Identifier }}.SuiteConfig,
{{- end }}
      BeforeAll: s.BeforeAll,
      AfterAll: s.AfterAll,
    })
{{- if and $fs.Suite.IsMethodParallel $fs.Suite.TestCases }}
    ƒfailed := &atomic.Bool{}
{{- end }}

{{ template "suiteMethods" (dict "Suite" $fs.Suite "FixtureOrder" $fs.FixtureOrder) }}
}
{{ end }}

{{- define "fixtureNode" -}}
            {
                Name: "{{ .Identifier }}",
{{- if .HasConfig }}
                Config: gotestruntime.WithFixtureDefaults(ƒcfg_{{ .Identifier }}),
                Budget: ƒcfg_{{ .Identifier }}.Timeout,
{{- else }}
                Config: gotest.DefaultFixtureConfig(),
{{- end }}
                Init: func() {
{{- if or .ParentFieldNames .SharedFixtures }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedType }}{
{{- range $parentID, $fieldName := .ParentFieldNames }}
                        {{ $fieldName }}: ƒ_{{ $parentID }},
{{- end }}
{{- range $sf := .SharedFixtures }}
                        {{ $sf.FieldName }}: ƒ_sf_{{ $sf.Identifier }},
{{- end }}
                    }
{{- else }}
                    ƒ_{{ .Identifier }} = &{{ .QualifiedType }}{}
{{- end }}
                },
                BeforeAll: func(ctx context.Context) error {
                    return ƒ_{{ .Identifier }}.BeforeAll(ctx)
                },
{{- if .AfterAll }}
                AfterAll: func(ctx context.Context) error {
                    return ƒ_{{ .Identifier }}.AfterAll(ctx)
                },
{{- end }}
{{- if .DependsOn }}
                DependsOn: []string{
{{- range $dep := .DependsOn }}
                    "{{ $dep }}",
{{- end }}
                },
{{- end }}
            },
{{- end -}}
