{{- /* Building blocks of the Test, Benchmark and Fuzz wrapper templates. */ -}}
{{- define "suiteWrapper" }}{{ $ts := . }}
type ƒƒ_GOTEST_{{ $ts.Identifier }} struct {
  {{ $ts.Identifier }}
}

func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeAll(it *gotest.T) { {{ if $ts.BeforeAll -}} ts.{{ $ts.Identifier }}.BeforeAll({{ if $ts.BeforeAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterAll(it *gotest.T) { {{ if $ts.AfterAll -}} ts.{{ $ts.Identifier }}.AfterAll({{ if $ts.AfterAll.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
{{- if $ts.HasReturningBeforeEach }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) {{ $ts.ContextTypeName }} { {{ if $ts.BeforeEach -}} return ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ else }}return nil {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T, ctx {{ $ts.ContextTypeName }}) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}, ctx) {{ end }}}
{{- else }}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) BeforeEach(it *gotest.T) { {{ if $ts.BeforeEach -}} ts.{{ $ts.Identifier }}.BeforeEach({{ if $ts.BeforeEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
func (ts *ƒƒ_GOTEST_{{ $ts.Identifier }}) AfterEach(it *gotest.T) { {{ if $ts.AfterEach -}} ts.{{ $ts.Identifier }}.AfterEach({{ if $ts.AfterEach.UsesStdlibT }}it.T(){{ else }}it{{ end }}) {{ end }}}
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
  {{ .Assign }}gotestruntime.OpenSuite({{ .TB }}, gotestruntime.Suite{
{{- if $ts.HasGuard }}
    Guard: s.{{ $ts.Identifier }}.SuiteGuard,
{{- end }}
{{- if $fx.Nodes }}
    Fixtures: func(tb testing.TB) {
      ƒ_holdFixtures(tb, func() time.Duration {
{{- if $ts.HasConfig }}
        return gotestruntime.WithSuiteDefaults((&{{ $ts.Identifier }}{}).SuiteConfig()).SetupTimeout
{{- else }}
        return gotest.DefaultSuiteConfig().SetupTimeout
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
  gotestruntime.RunMethods(t, ƒcfg, ƒbudget, gotestruntime.Methods{
{{- if $ts.IsMethodParallel }}
    Parallel: true,
{{- end }}
{{- if $hooks }}
    Fixtures: []gotestruntime.EachHooks{
{{- range $fix := $fixtures }}{{ if or $fix.BeforeEach $fix.AfterEach }}
      {
        Name: "{{ $fix.Identifier }}",
{{- if $fix.BeforeEach }}
        Before: func(ctx context.Context) error { return ƒ_{{ $fix.Identifier }}.BeforeEach(ctx) },
{{- end }}
{{- if $fix.AfterEach }}
        After: func(ctx context.Context) error { return ƒ_{{ $fix.Identifier }}.AfterEach(ctx) },
{{- end }}
      },
{{- end }}{{ end }}
    },
{{- end }}
    Cases: []gotestruntime.Method{
{{- range $tc := $ts.TestCases }}
{{- $t := "t" }}{{ if $tc.UsesStdlibT }}{{ $t = "t.T()" }}{{ end }}
{{- $ctx := "" }}{{ if $ts.HasReturningBeforeEach }}{{ $ctx = ", ctx" }}{{ end }}
      {Name: "{{ $tc.Identifier }}", Run: func(ttt *gotest.T, run func(func())) {
{{- if $ts.HasReturningBeforeEach }}
        ctx := s.BeforeEach(ttt)
        defer gotestruntime.AfterEach(func() { s.AfterEach(ttt, ctx) })
{{- else }}
        defer gotestruntime.AfterEach(func() { s.AfterEach(ttt) })
        s.BeforeEach(ttt)
{{- end }}
        run(func() {
{{- if $tc.IsAsync }}
          gotestruntime.AwaitDoneAt(ttt, "{{ $tc.Identifier }}", {{ printf "%q" ($tc.DeclaredAt $ts.FileSet) }}, func(done func()) {
            ƒƒ_GOTEST_exec(func(t *gotest.T) { s.{{ $tc.Identifier }}({{ $t }}{{ $ctx }}, done) }, ttt)
          })
{{- else if or $tc.UsesStdlibT $ts.HasReturningBeforeEach }}
          ƒƒ_GOTEST_exec(func(t *gotest.T) { s.{{ $tc.Identifier }}({{ $t }}{{ $ctx }}) }, ttt)
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
func Test{{ .Identifier }}(t *testing.T) {
  t.Skipf("test suite was excluded by user")
}
{{- end }}
