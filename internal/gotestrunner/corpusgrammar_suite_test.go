package gotestrunner_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CorpusGrammarTestSuite: a corpus file is read in the grammar Go's fuzz
// encoder writes, and nothing else.
type CorpusGrammarTestSuite struct{}

func (s *CorpusGrammarTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func writeCorpusBody(t *gotest.T, body string) string {
	path := filepath.Join(t.TempDir(), "entry")
	gotest.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func (s *CorpusGrammarTestSuite) TestEncoderSpellings(t *gotest.T) {
	for sub, c := range gotest.Each(t, []struct {
		Desc   string
		line   string
		typ    string
		corpus string
	}{
		{Desc: "int", line: "int(-3)", typ: "int", corpus: "int(-3)"},
		{Desc: "int8", line: "int8(-128)", typ: "int8", corpus: "int8(-128)"},
		{Desc: "int16", line: "int16(300)", typ: "int16", corpus: "int16(300)"},
		{Desc: "int32", line: "int32(-1)", typ: "int32", corpus: "int32(-1)"},
		{Desc: "int64", line: "int64(9223372036854775807)", typ: "int64", corpus: "int64(9223372036854775807)"},
		{Desc: "uint", line: "uint(7)", typ: "uint", corpus: "uint(7)"},
		{Desc: "uint16", line: "uint16(65535)", typ: "uint16", corpus: "uint16(65535)"},
		{Desc: "uint32", line: "uint32(1)", typ: "uint32", corpus: "uint32(1)"},
		{Desc: "uint64", line: "uint64(18446744073709551615)", typ: "uint64", corpus: "uint64(18446744073709551615)"},
		{Desc: "hex int", line: "int(0x1f)", typ: "int", corpus: "int(0x1f)"},
		{Desc: "float64", line: "float64(1.5)", typ: "float64", corpus: "float64(1.5)"},
		{Desc: "float32", line: "float32(-0)", typ: "float32", corpus: "float32(-0)"},
		{Desc: "float64 +Inf", line: "float64(+Inf)", typ: "float64", corpus: "float64(+Inf)"},
		{Desc: "float64 -Inf", line: "float64(-Inf)", typ: "float64", corpus: "float64(-Inf)"},
		{Desc: "float64 NaN", line: "float64(NaN)", typ: "float64", corpus: "float64(NaN)"},
		{Desc: "float64 bits", line: "math.Float64frombits(0x7ff8000000000001)", typ: "float64", corpus: "math.Float64frombits(0x7ff8000000000001)"},
		{Desc: "float32 bits", line: "math.Float32frombits(0x7fc00001)", typ: "float32", corpus: "math.Float32frombits(0x7fc00001)"},
		{Desc: "string", line: `string("a@\x00")`, typ: "string", corpus: `string("a@\x00")`},
		{Desc: "raw string", line: "string(`a`)", typ: "string", corpus: "string(`a`)"},
		{Desc: "byte slice", line: `[]byte("\xff")`, typ: "[]byte", corpus: `[]byte("\xff")`},
		{Desc: "bool", line: "bool(true)", typ: "bool", corpus: "bool(true)"},
		{Desc: "rune", line: `rune('é')`, typ: "rune", corpus: `rune('é')`},
		{Desc: "rune as int", line: "rune(97)", typ: "rune", corpus: "rune(97)"},
		{Desc: "byte", line: `byte('\x00')`, typ: "byte", corpus: `byte('\x00')`},
		{Desc: "byte as int", line: "byte(5)", typ: "byte", corpus: "byte(5)"},
	}) {
		args, err := gotestrunner.ParseCorpusFile(writeCorpusBody(sub, "go test fuzz v1\n"+c.line+"\n"))
		gotest.NoError(sub, err)
		gotest.Len(sub, args, 1)
		gotest.Equal(sub, c.typ, args[0].TypeName)
		gotest.Equal(sub, c.corpus, args[0].Corpus())
	}
}

func (s *CorpusGrammarTestSuite) TestRefusals(t *gotest.T) {
	for sub, c := range gotest.Each(t, []struct {
		Desc string
		line string
	}{
		{Desc: "a call", line: `string(os.Getenv("HOME"))`},
		{Desc: "a call in a conversion", line: `int(len("abc"))`},
		{Desc: "a func literal", line: `int(func() int { return 1 }())`},
		{Desc: "an identifier", line: "string(home)"},
		{Desc: "an identifier for a bool", line: "bool(yes)"},
		{Desc: "an operator", line: "int(1 + 2)"},
		{Desc: "a unary plus", line: "int(+1)"},
		{Desc: "a negated string", line: `string(-"a")`},
		{Desc: "a composite literal", line: "[]byte{1, 2}"},
		{Desc: "a struct literal", line: "struct{X int}{1}"},
		{Desc: "a selector type", line: "time.Duration(5)"},
		{Desc: "an unknown type", line: "complex128(1)"},
		{Desc: "two arguments", line: "int(1, 2)"},
		{Desc: "a spread argument", line: "string(s...)"},
		{Desc: "an array type", line: `[4]byte("abcd")`},
		{Desc: "a string for an int", line: `int("1")`},
		{Desc: "an int for a string", line: "string(1)"},
		{Desc: "a byte slice from an int", line: "[]byte(1)"},
		{Desc: "an out-of-range int8", line: "int8(300)"},
		{Desc: "a negative uint", line: "uint(-1)"},
		{Desc: "a float for an int", line: "int(1.5)"},
		{Desc: "an imaginary float", line: "float64(1i)"},
		{Desc: "a wide rune for a byte", line: `byte('世')`},
		{Desc: "an unknown math function", line: "math.Sqrt(2)"},
		{Desc: "a bare identifier", line: "hello"},
		{Desc: "stray text after a literal", line: `string("a") trailing`},
		{Desc: "a statement", line: `string("a"); os.Exit(1)`},
	}) {
		path := writeCorpusBody(sub, "go test fuzz v1\nstring(\"fine\")\n"+c.line+"\n")
		_, err := gotestrunner.ParseCorpusFile(path)
		gotest.ErrorContains(sub, err, path+":3: ")
	}
}

func (s *CorpusGrammarTestSuite) TestHeader(t *gotest.T) {
	t.It("refuses a file without the header, at line 1", func(it *gotest.T) {
		path := writeCorpusBody(it, "go test fuzz v2\nstring(\"a\")\n")
		_, err := gotestrunner.ParseCorpusFile(path)
		gotest.ErrorContains(it, err, path+":1: ")
	})
	t.It("reads a header ended by CRLF", func(it *gotest.T) {
		path := writeCorpusBody(it, "go test fuzz v1\r\nstring(\"a\")\r\n")
		args, err := gotestrunner.ParseCorpusFile(path)
		gotest.NoError(it, err)
		gotest.Len(it, args, 1)
	})
	t.It("skips blank lines and counts them", func(it *gotest.T) {
		path := writeCorpusBody(it, "go test fuzz v1\n\nstring(\"a\")\n\nhome\n")
		_, err := gotestrunner.ParseCorpusFile(path)
		gotest.ErrorContains(it, err, path+":5: ")
	})
}
