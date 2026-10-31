{{- /* Building blocks of the Test, Benchmark and Fuzz wrapper templates. */ -}}
{{- define "suiteWrapper" }}{{ $ts := .Suite }}
type ƒƒ_GOTEST_{{ $ts.Identifier }} struct {
  {{ $ts.Identifier }}
}

func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeAll(it *ƒgotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll({{ if $ts.BeforeAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterAll(it *ƒgotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll({{ if $ts.AfterAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- if $ts.HasReturningBeforeEach }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *ƒgotest.T) {{ index $.ContextTypes $ts.Identifier }} { {{ if $ts.BeforeEach -}} return ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ else }}return nil {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *ƒgotest.T, ctx {{ index $.ContextTypes $ts.Identifier }}) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}, ctx) {{ end }}}
{{- else }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *ƒgotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *ƒgotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- end }}
{{- end }}

{{- /*
  Takes dict "Suite", "Fixtures" (its SuiteFixtureSet), "TB", the wrapper's
  testing value, and "Assign", what receives OpenSuite's results. Every wrapper
  opens its suite here, so each holds the same fixtures, wires the same fields,
  and does so only once the guard let the suite run.
*/ -}}
{{- define "suiteFrame" }}{{ $ts := .Suite }}{{ $fx := .Fixtures }}
  s := &ƒƒ_GOTEST_{{ $ts.Identifier }}{}
  {{ .Assign }}ƒgotestruntime.OpenSuite({{ .TB }}, ƒgotestruntime.Suite{
{{- if $ts.HasGuard }}
    Guard: s.{{ $ts.Identifier }}.SuiteGuard,
{{- end }}
{{- if $fx.Nodes }}
    Fixtures: func(tb ƒtesting.TB) {
      ƒ_holdFixtures(tb, func() ƒtime.Duration {
{{- if $ts.HasConfig }}
        return ƒgotestruntime.WithSuiteDefaults((&{{ $ts.Identifier }}{}).SuiteConfig()).SetupTimeout
{{- else }}
        return ƒgotestruntime.WithSuiteDefaults(ƒgotest.SuiteConfig{}).SetupTimeout
{{- end }}
      }{{ range $n := $fx.Nodes }}, ƒ_node_{{ $n }}{{ end }})
{{- range $f := $fx.Fields }}
      s.{{ $ts.Identifier }}.{{ $f.Name }} = {{ $f.Var }}
{{- end }}
    },
{{- end }}
{{- if $ts.HasConfig }}
    Config: s.{{ $ts.Identifier }}.SuiteConfig,
{{- end }}
    BeforeAll: s.BeforeAll,
    AfterAll: s.AfterAll,
  })
{{- end }}

{{- /*
  Takes dict "Suite" and, for a fixture-bound suite, "FixtureOrder". The
  runtime owns the loop; each case keeps what needs the suite's static types:
  its BeforeEach and AfterEach, with a returned context, and the method call.
  Every call goes through the ƒƒ_GOTEST_exec trampoline, where assertion call
  sites stop.
*/ -}}
{{- define "suiteMethods" }}{{ $ts := .Suite }}{{ $fixtures := .FixtureOrder }}
{{- $hooks := false }}{{ range $fix := $fixtures }}{{ if or $fix.BeforeEach $fix.AfterEach }}{{ $hooks = true }}{{ end }}{{ end }}
  ƒgotestruntime.RunMethods(t, ƒcfg, ƒbudget, ƒgotestruntime.Methods{
{{- if $ts.IsMethodParallel }}
    Parallel: true,
{{- end }}
{{- if $hooks }}
    Fixtures: []ƒgotestruntime.EachHooks{
{{- range $fix := $fixtures }}{{ if or $fix.BeforeEach $fix.AfterEach }}
      {
        Name: "{{ $fix.Identifier }}",
{{- if $fix.BeforeEach }}
        Before: func(ctx ƒcontext.Context) error { return ƒ_{{ $fix.Identifier }}.BeforeEach(ctx) },
        BeforeAt: {{ printf "%q" $fix.BeforeEachAt }},
{{- end }}
{{- if $fix.AfterEach }}
        After: func(ctx ƒcontext.Context) error { return ƒ_{{ $fix.Identifier }}.AfterEach(ctx) },
        AfterAt: {{ printf "%q" $fix.AfterEachAt }},
{{- end }}
      },
{{- end }}{{ end }}
    },
{{- end }}
    Cases: []ƒgotestruntime.Method{
{{- range $tc := $ts.TestCases }}
{{- $t := "t" }}{{ if $tc.UsesStdlibT }}{{ $t = "t.T()" }}{{ end }}
{{- $ctx := "" }}{{ if $ts.HasReturningBeforeEach }}{{ $ctx = ", ctx" }}{{ end }}
      {Name: "{{ $tc.Identifier }}", Run: func(ttt *ƒgotest.T, run func(func())) {
{{- if $ts.HasReturningBeforeEach }}
        ctx := s.BeforeEach(ttt)
        defer ƒgotestruntime.AfterEach(func() { s.AfterEach(ƒgotestruntime.AfterEachT(ttt), ctx) })
{{- else }}
        defer ƒgotestruntime.AfterEach(func() { s.AfterEach(ƒgotestruntime.AfterEachT(ttt)) })
        s.BeforeEach(ttt)
{{- end }}
        run(func() {
{{- if or $tc.UsesStdlibT $ts.HasReturningBeforeEach }}
          ƒƒ_GOTEST_exec(func(t *ƒgotest.T) { s.{{ $tc.Identifier }}({{ $t }}{{ $ctx }}) }, ttt)
{{- else }}
          ƒƒ_GOTEST_exec(s.{{ $tc.Identifier }}, ttt)
{{- end }}
        })
      }},
{{- end }}
    },
  })
{{- end }}

{{- /* Skipped TestX of an excluded suite, or of one whose every Test method is excluded. */ -}}
{{- define "excludedSuite" }}
func Test{{ .Identifier }}(t *ƒtesting.T) {
  t.Skipf("test suite was excluded by user")
}
{{- end }}
