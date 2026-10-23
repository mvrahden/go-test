package assertions_test

import (
	"errors"
	"io"
	"io/fs"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AssertionTypingTestSuite: assertions take the types they are given at face
// value and say what they cannot work with.
type AssertionTypingTestSuite struct{}

func (s *AssertionTypingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type pattern string

type note string

func (s *AssertionTypingTestSuite) TestRegexp(t *gotest.T) {
	t.When("Regexp is given a pattern of a named string type", func(w *gotest.T) {
		w.It("compiles and matches it like a string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, pattern(`^a+$`), "aaa") })
			gotest.False(it, m.Failed(), m.Message())
			miss := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, pattern(`^a+$`), "b") })
			gotest.True(it, miss.Failed())
		})
	})
}

func (s *AssertionTypingTestSuite) TestContains(t *gotest.T) {
	t.When("Contains is given a needle no element of the haystack could be", func(w *gotest.T) {
		w.It("fails naming the haystack's type and the needle's", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2}, "x") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "[]int cannot hold a value of type string")
			n := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1}, 7) })
			gotest.True(it, n.Failed())
			gotest.Contains(it, n.Message(), "map[string]int cannot hold a key of type int")
		})
	})
	t.When("NotContains is given a needle that does not implement the interface the slice's elements are", func(w *gotest.T) {
		w.It("fails naming the slice's type and the needle's", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []error{io.EOF}, "EOF") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "[]error cannot hold a value of type string")
			held := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []error{io.EOF}, io.EOF) })
			gotest.False(it, held.Failed(), held.Message())
		})
	})
	t.When("Contains is given a number of another type than the slice's numeric elements", func(w *gotest.T) {
		w.It("the failure names the type to convert it to", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int64{1, 2}, 3) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "[]int64 cannot hold a value of type int (convert it to int64)")
		})
	})
	t.When("Contains is given a number the slice's numeric elements cannot hold exactly", func(w *gotest.T) {
		w.It("the failure names no type to convert it to", func(it *gotest.T) {
			for _, m := range []*gotest.R{
				gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{3}, 3.7) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int8{44}, 300) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []uint{1}, -1) }),
			} {
				gotest.True(it, m.Failed())
				gotest.Contains(it, m.Message(), "cannot hold a value of type")
				gotest.NotContains(it, m.Message(), "convert it")
			}
		})
	})
	t.When("Contains is given a rune, a byte or a byte slice for a string", func(w *gotest.T) {
		w.It("fails naming the string type to convert it to", func(it *gotest.T) {
			for _, c := range []struct {
				m    *gotest.R
				want string
			}{
				{gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "abc", 'x') }), "string cannot hold a value of type int32 (convert it to string)"},
				{gotest.Record(func(r *gotest.R) { gotest.NotContains(r, "abc", byte('x')) }), "string cannot hold a value of type uint8 (convert it to string)"},
				{gotest.Record(func(r *gotest.R) { gotest.Contains(r, "abc", []byte("b")) }), "string cannot hold a value of type []uint8 (convert it to string)"},
			} {
				gotest.True(it, c.m.Failed())
				gotest.Contains(it, c.m.Message(), c.want)
			}
		})
	})
	t.When("Contains is given a needle that is not comparable for a map keyed by an interface type", func(w *gotest.T) {
		w.It("fails naming the needle's type", func(it *gotest.T) {
			var m *gotest.R
			panicked := true
			func() {
				defer func() { _ = recover() }()
				m = gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[any]int{1: 1}, []int{1}) })
				panicked = false
			}()
			gotest.False(it, panicked, "Contains panicked")
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "cannot hold a key of type []int: it is not comparable")
		})
	})
	t.When("Contains is asked for a nil key in a map keyed by an interface or pointer type", func(w *gotest.T) {
		w.It("finds the key when present and reports it absent otherwise", func(it *gotest.T) {
			for _, m := range []*gotest.R{
				gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[any]int{nil: 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[*int]int{nil: 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[any]int{"a": 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[*int]int{new(int): 1}, nil) }),
			} {
				gotest.False(it, m.Failed(), m.Message())
			}
			miss := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[any]int{"a": 1}, nil) })
			gotest.True(it, miss.Failed())
			gotest.Contains(it, miss.Message(), "does not contain")
		})
	})
	t.When("Contains is asked for a nil key in a map whose key type cannot be nil", func(w *gotest.T) {
		w.It("fails naming the map's type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"": 1}, nil) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "map[string]int cannot hold a nil key")
		})
	})
	t.When("Contains is asked for a nil element in a slice of a pointer type", func(w *gotest.T) {
		w.It("finds the nil element when present and reports it absent otherwise", func(it *gotest.T) {
			found := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []*int{new(int), nil}, nil) })
			gotest.False(it, found.Failed(), found.Message())
			absent := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []*int{new(int)}, nil) })
			gotest.False(it, absent.Failed(), absent.Message())
			miss := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []*int{new(int)}, nil) })
			gotest.True(it, miss.Failed())
			gotest.Contains(it, miss.Message(), "does not contain")
		})
	})
	t.When("Contains is asked for a nil element in a slice whose element type cannot be nil", func(w *gotest.T) {
		w.It("fails naming the slice's type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, []int{1}, nil) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "[]int cannot hold a nil element")
		})
	})
	t.When("Contains is asked for a nil substring of a string", func(w *gotest.T) {
		w.It("fails naming the string's type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, note("abc"), nil) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "assertions_test.note cannot hold a nil substring")
		})
	})
	t.When("Contains compares named string kinds", func(w *gotest.T) {
		w.It("checks the substring as it does for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, note("hello world"), note("lo w")) })
			gotest.False(it, m.Failed(), m.Message())
			miss := gotest.Record(func(r *gotest.R) { gotest.Contains(r, note("hello"), "zzz") })
			gotest.True(it, miss.Failed())
			gotest.Contains(it, miss.Message(), "does not contain")
		})
	})
}

func (s *AssertionTypingTestSuite) TestInDelta(t *gotest.T) {
	t.When("InDelta is given a NaN delta", func(w *gotest.T) {
		w.It("fails: no distance is within NaN", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 1.0, 1.0, math.NaN()) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "delta is NaN")
		})
	})
	t.When("InDelta compares two equal infinities", func(w *gotest.T) {
		w.It("they are within any delta", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.Inf(1), math.Inf(1), 0) })
			gotest.False(it, m.Failed(), m.Message())
			opposite := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.Inf(1), math.Inf(-1), 1e300) })
			gotest.True(it, opposite.Failed())
		})
	})
	t.When("InDelta compares integers beyond float64's exact range", func(w *gotest.T) {
		w.It("9007199254740993 is not within a delta of 0 of 9007199254740992", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, int64(9007199254740993), int64(9007199254740992), 0) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "= 1 exceeds delta 0")
		})
	})
	t.When("InDelta compares the smallest and the largest int64", func(w *gotest.T) {
		w.It("measures their full distance, not one that wrapped around", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, int64(math.MinInt64), int64(math.MaxInt64), 1) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "= 18446744073709551615 exceeds delta 1")
		})
	})
}

func (s *AssertionTypingTestSuite) TestMessages(t *gotest.T) {
	t.When("an assertion's first extra argument is not a string", func(w *gotest.T) {
		w.It("every extra argument is in the message", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false, 42, "retries left") })
			gotest.Contains(it, m.Message(), "42 retries left")
		})
	})
	t.When("an assertion's first extra argument is of a named string type", func(w *gotest.T) {
		w.It("formats the arguments after it", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false, note("attempt %d of %d"), 2, 3) })
			gotest.Contains(it, m.Message(), "attempt 2 of 3")
		})
	})
	t.When("ErrorAs is given an error of another type", func(w *gotest.T) {
		w.It("names the type it expected", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { _ = gotest.ErrorAs[*fs.PathError](r, errors.New("plain")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "is of type *fs.PathError")
			iface := gotest.Record(func(r *gotest.R) {
				_ = gotest.ErrorAs[interface {
					error
					Timeout() bool
				}](r, errors.New("plain"))
			})
			gotest.Contains(it, iface.Message(), "is of type interface {")
		})
	})
}

func (s *AssertionTypingTestSuite) TestJSONEq(t *gotest.T) {
	t.When("JSONEq compares integers beyond float64's exact range", func(w *gotest.T) {
		w.It("tells 9007199254740993 from 9007199254740992", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":9007199254740993}`, `{"n":9007199254740992}`) })
			gotest.True(it, m.Failed())
			same := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":9007199254740993}`, `{"n":9007199254740993}`) })
			gotest.False(it, same.Failed(), same.Message())
		})
	})
	t.When("JSONEq is given a value followed by a stray } or ]", func(w *gotest.T) {
		w.It("fails: the input is not valid JSON", func(it *gotest.T) {
			for _, pair := range [][2]string{{`{"a":1}}`, `{"a":1}`}, {`[1]]`, `[1]`}} {
				m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, pair[0], pair[1]) })
				gotest.True(it, m.Failed(), pair[0])
				gotest.Contains(it, m.Message(), "could not normalize expected", pair[0])
				gotest.Contains(it, m.Message(), "trailing data after the value", pair[0])
			}
		})
	})
	t.When("JSONEq compares an integer with its decimal spelling", func(w *gotest.T) {
		w.It("1 equals 1.0", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":1}`, `{"n":1.0}`) })
			gotest.False(it, m.Failed(), m.Message())
			e := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1e2]`, `[100]`) })
			gotest.False(it, e.Failed(), e.Message())
		})
	})
	t.When("JSONEq compares numbers whose exponents are spelled differently, however large", func(w *gotest.T) {
		w.It("1e10000000 equals 10e9999999", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1e10000000]`, `[10e9999999]`) })
			gotest.False(it, m.Failed(), m.Message())
			apart := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1e10000000]`, `[1e10000001]`) })
			gotest.True(it, apart.Failed())
		})
	})
	t.When("JSONEq compares -0 with 0", func(w *gotest.T) {
		w.It("they are equal", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":-0}`, `{"n":0.0}`) })
			gotest.False(it, m.Failed(), m.Message())
		})
	})
}

type lines []string

func (v lines) GoString() string { return strings.Join(v, "\n") }

type text string

func (v text) GoString() string { return string(v) }

type address struct{ City, Zip string }

type customer struct {
	Name    string
	Address address
}

type owner struct{ Name string }

type item struct {
	Name  string
	Owner *owner
}

type account struct {
	Holder string
	Note   string
	pin    int
}

func (s *AssertionTypingTestSuite) TestDiffs(t *gotest.T) {
	t.When("Equal fails on values whose rendering spans lines", func(w *gotest.T) {
		w.It("the message carries a line diff: hunks of changes with three lines of context, without file headers", func(it *gotest.T) {
			before := lines{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
			after := append(lines(nil), before...)
			after[5] = "F"
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, before, after) })
			gotest.True(it, m.Failed())
			_, diff, _ := strings.Cut(m.Message(), "  diff (-expected +actual):\n")
			gotest.Equal(it, "     c\n     d\n     e\n    -f\n    +F\n     g\n     h\n     i", diff, "the hunk holds three lines of context on each side")
			gotest.NotContains(it, m.Message(), "@@")
			gotest.NotContains(it, m.Message(), "--- expected")
		})
	})
	t.When("Equal fails on multi-line values that differ only in a final newline", func(w *gotest.T) {
		w.It("the diff marks the side that does not end in one", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, text("a\nb\n"), text("a\nb")) })
			gotest.True(it, m.Failed())
			gotest.Regexp(it, `\+b\n +\\ No newline at end of file`, m.Message())
		})
	})
	t.When("Equal fails on multi-line values whose changed last line is longer than 80 bytes and ends in a newline on one side only", func(w *gotest.T) {
		w.It("the diff shows that line around its first difference and marks the side without the newline", func(it *gotest.T) {
			long := strings.Repeat("x", 100)
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, text("a\n"+long+"1\n"), text("a\n"+long+"2")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n    -…"+strings.Repeat("x", 30)+"1\n    +…"+strings.Repeat("x", 30)+"2\n    \\ No newline at end of file\n")
			gotest.NotContains(it, m.Message(), "\x00")
		})
	})
	t.When("Equal fails on structs that differ in one nested field", func(w *gotest.T) {
		w.It("the diff marks that field's line and no other", func(it *gotest.T) {
			a := customer{Name: "Ada Lovelace, Countess of Lovelace", Address: address{City: "London", Zip: "SW1Y 4JH"}}
			b := a
			b.Address.Zip = "SW1Y 4JJ"
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Equal(it, []string{`    -    Zip: "SW1Y 4JH",`, `    +    Zip: "SW1Y 4JJ",`}, markedLines(m.Message()))
		})
	})
	t.When("Equal fails on slices whose elements hold distinct pointers to equal values and one element differs", func(w *gotest.T) {
		w.It("the diff marks a line of that element only", func(it *gotest.T) {
			a := []item{{"first", &owner{"o"}}, {"second", &owner{"o"}}}
			b := []item{{"first", &owner{"o"}}, {"SECOND", &owner{"o"}}}
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Equal(it, []string{`    -    Name: "second",`, `    +    Name: "SECOND",`}, markedLines(m.Message()))
			gotest.NotContains(it, m.Message(), "0x")
		})
	})
	t.When("Equal fails on structs that differ only in an unexported field", func(w *gotest.T) {
		w.It("the diff marks that field's line", func(it *gotest.T) {
			a := account{Holder: "Ada Lovelace", Note: "a note long enough to need more than one line", pin: 1234}
			b := a
			b.pin = 4321
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Equal(it, []string{"    -  pin: 1234,", "    +  pin: 4321,"}, markedLines(m.Message()))
		})
	})
	t.When("Equal fails on strings that span lines", func(w *gotest.T) {
		w.It("the diff marks the changed lines of text", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, "one\ntwo\nthree\n", "one\n2\nthree\n") })
			gotest.True(it, m.Failed())
			gotest.Equal(it, []string{`    -  "two\n" +`, `    +  "2\n" +`}, markedLines(m.Message()))
		})
	})
}

func (s *AssertionTypingTestSuite) TestDiffShapes(t *gotest.T) {
	t.When("Equal fails on values that differ in a field of an element", func(w *gotest.T) {
		w.It("the hunk is led by the path of its first changed line, such as at [1].Name:", func(it *gotest.T) {
			a := []item{{"first", &owner{"o"}}, {"second", &owner{"o"}}}
			b := []item{{"first", &owner{"o"}}, {"SECOND", &owner{"o"}}}
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "  diff (-expected +actual):\n    at [1].Name:\n")
		})
	})
	t.When("Equal fails on slices of structs one of which lacks an element", func(w *gotest.T) {
		w.It("the diff marks that element from its opening line to its closing one", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, []pt{{1, 0}, {2, 0}, {3, 0}, {4, 0}}, []pt{{1, 0}, {3, 0}, {4, 0}}) })
			gotest.True(it, m.Failed())
			gotest.Equal(it, []string{"    -  assertions_test.pt{", "    -    X: 2,", "    -    Y: 0,", "    -  },"}, markedLines(m.Message()))
		})
	})
	t.When("Equal fails on maps whose keys render alike, such as distinct pointers to equal values", func(w *gotest.T) {
		w.It("entries are ordered by their values and only the changed entry is marked, the same way every time", func(it *gotest.T) {
			p, q, r := &owner{"o"}, &owner{"o"}, &owner{"o"}
			a := map[*owner]int{p: 1, q: 2, r: 3}
			b := map[*owner]int{p: 1, q: 2, r: 4}
			message := func() string { return gotest.Record(func(rec *gotest.R) { gotest.Equal(rec, a, b) }).Message() }
			first := message()
			gotest.Equal(it, []string{`    -  &assertions_test.owner{Name:"o"}: 3,`, `    +  &assertions_test.owner{Name:"o"}: 4,`}, markedLines(first))
			for range 20 {
				gotest.Equal(it, first, message())
			}
		})
	})
	t.When("Equal fails on byte slices of UTF-8 text that span lines", func(w *gotest.T) {
		w.It(`they render as []byte("…") split at their newlines and the diff marks the changed lines`, func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, []byte("one\ntwo\n"), []byte("one\n2\n")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "  expected: []byte(\"one\\n\" +\n              \"two\\n\")\n")
			gotest.Equal(it, []string{`    -  "two\n")`, `    +  "2\n")`}, markedLines(m.Message()))
		})
	})
	t.When("Equal fails on byte slices that are not UTF-8", func(w *gotest.T) {
		w.It("they render as rows of eight hex bytes", func(it *gotest.T) {
			a := []byte{0xff, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			b := append([]byte(nil), a...)
			b[16] = 0xee
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n       0xff, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,\n       0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,\n    -  0x10,\n    +  0xee,\n")
		})
	})
	t.When("Equal fails on byte slices that are not UTF-8 and differ inside a row", func(w *gotest.T) {
		w.It("the hunk names the first differing byte, such as at [12]:, not the start of its row", func(it *gotest.T) {
			a := []byte{0xff, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
			b := append([]byte(nil), a...)
			b[12] = 0xee
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n    at [12]:\n")
		})
	})
	t.When("Equal fails on structs that hold a sync.Mutex and differ in another field", func(w *gotest.T) {
		w.It("the mutex renders as sync.Mutex{…}", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, &guarded{N: 1}, &guarded{N: 2}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "expected: &assertions_test.guarded{Mutex:sync.Mutex{…}, N:1}")
		})
	})
	t.When("Equal fails on values holding unsigned integers", func(w *gotest.T) {
		w.It("they render in decimal, a uintptr in hex", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, counts{300, 16}, counts{301, 16}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "expected: assertions_test.counts{U:300, P:0x10}\n  actual:   assertions_test.counts{U:301, P:0x10}")
		})
	})
	t.When("Equal fails on values whose one changed line is longer than 80 bytes", func(w *gotest.T) {
		w.It("the diff shows that line around its first difference with a caret under it", func(it *gotest.T) {
			long := strings.Repeat("x", 200)
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, address{long + "a", "N1"}, address{long + "b", "N1"}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n    -  …"+strings.Repeat("x", 30)+"a\",\n    +  …"+strings.Repeat("x", 30)+"b\",\n"+strings.Repeat(" ", 38)+"^\n")
		})
	})
	t.When("Equal fails on values nested more than 32 levels deep", func(w *gotest.T) {
		w.It("lines deeper than 32 levels keep that level's indent and name their depth", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, chain(40, 1), chain(40, 2)) })
			gotest.True(it, m.Failed())
			in := strings.Repeat(" ", 64)
			gotest.Contains(it, m.Message(), "‹depth 40› Next: (*assertions_test.link)(nil),\n    -"+in+"V: 1,\n    +"+in+"V: 2,\n")
		})
	})
	t.When("Equal fails on values whose renderings pass 50,000 lines and agree up to there", func(w *gotest.T) {
		w.It("the message says where they are cut and names the path of their first difference", func(it *gotest.T) {
			a, b := make([]int, 60000), make([]int, 60000)
			b[59999] = 1
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "the renderings are cut at 50000 lines; the values first differ at [59999]")
		})
	})
}

type pt struct{ X, Y int }

type guarded struct {
	sync.Mutex
	N int
}

type counts struct {
	U uint16
	P uintptr
}

type link struct {
	Next *link
	V    int
}

// chain is n links, the last holding v.
func chain(n, v int) *link {
	head := &link{V: v}
	for range n - 1 {
		head = &link{Next: head}
	}
	return head
}

// markedLines are the diff's - and + lines.
func markedLines(msg string) []string {
	var out []string
	for _, l := range strings.Split(msg, "\n") {
		if strings.HasPrefix(l, "    -") || strings.HasPrefix(l, "    +") {
			out = append(out, l)
		}
	}
	return out
}

func (s *AssertionTypingTestSuite) TestMismatchesWithoutADiff(t *gotest.T) {
	t.When("Equal fails on values that each render on one short line", func(w *gotest.T) {
		w.It("the message shows both values and no diff", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, address{"London", "N1"}, address{"London", "N2"}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "  expected: assertions_test.address{City:\"London\", Zip:\"N1\"}\n  actual:   assertions_test.address{City:\"London\", Zip:\"N2\"}")
			gotest.NotContains(it, m.Message(), "diff")
		})
	})
	t.When("Equal fails on two long single-line strings", func(w *gotest.T) {
		w.It("the message names the byte offset of the first difference and shows both strings around it", func(it *gotest.T) {
			a := strings.Repeat("x", 100) + "a" + strings.Repeat("y", 100)
			b := strings.Repeat("x", 100) + "b" + strings.Repeat("y", 100)
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "strings differ at byte 100")
			gotest.Contains(it, m.Message(), `expected: …"`+strings.Repeat("x", 30)+"a"+strings.Repeat("y", 29)+`"…`)
			gotest.Contains(it, m.Message(), `actual:   …"`+strings.Repeat("x", 30)+"b"+strings.Repeat("y", 29)+`"…`)
			gotest.NotContains(it, m.Message(), strings.Repeat("x", 31))
		})
	})
	t.When("Equal fails on two long single-line byte slices of UTF-8 text", func(w *gotest.T) {
		w.It("the message names the byte offset of the first difference and shows both around it", func(it *gotest.T) {
			a := []byte(strings.Repeat("x", 100) + "a" + strings.Repeat("y", 100))
			b := []byte(strings.Repeat("x", 100) + "b" + strings.Repeat("y", 100))
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "byte slices differ at byte 100 (expected 201 bytes, actual 201)")
			gotest.Contains(it, m.Message(), `expected: …"`+strings.Repeat("x", 30)+"a"+strings.Repeat("y", 29)+`"…`)
			gotest.Contains(it, m.Message(), `actual:   …"`+strings.Repeat("x", 30)+"b"+strings.Repeat("y", 29)+`"…`)
		})
	})
	t.When("Equal fails on two long strings with wide characters before their first difference", func(w *gotest.T) {
		w.It("the caret sits under the difference, a wide character counting two columns", func(it *gotest.T) {
			a := strings.Repeat("語", 30) + "a" + strings.Repeat("y", 10)
			b := strings.Repeat("語", 30) + "b" + strings.Repeat("y", 10)
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, a, b) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n  actual:   …\""+strings.Repeat("語", 10)+"b"+strings.Repeat("y", 10)+"\"\n"+strings.Repeat(" ", 34)+"^")
		})
	})
	t.When("Equal fails on values that each render on one line, one longer than 160 bytes, that first differ past byte 160", func(w *gotest.T) {
		w.It("the message shows both around their first difference with a caret under it", func(it *gotest.T) {
			x, y := strings.Repeat("y", 200)+"1", strings.Repeat("y", 200)+"2"
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, &x, &y) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "\n  expected: …"+strings.Repeat("y", 30)+"1\")\n  actual:   …"+strings.Repeat("y", 30)+"2\")\n"+strings.Repeat(" ", 43)+"^")
		})
	})
	t.When("Equal fails on short values of different dynamic types", func(w *gotest.T) {
		w.It("the message names both types and shows each value in its type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal[any](r, 1, float64(1)) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "Equal failed:\n  types differ: int vs float64\n  expected: int(1)\n  actual:   float64(1)")
		})
	})
	t.When("Equal fails on two long single-line strings of different types inside any", func(w *gotest.T) {
		w.It("the message names both types, then the byte offset of the first difference and both strings around it", func(it *gotest.T) {
			l := strings.Repeat("y", 200)
			m := gotest.Record(func(r *gotest.R) { gotest.Equal[any](r, l+"1", note(l+"2")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "types differ: string vs assertions_test.note\n  strings differ at byte 200 (expected 201 bytes, actual 201)\n  expected: …\""+strings.Repeat("y", 30)+"1\"\n  actual:   …\""+strings.Repeat("y", 30)+"2\"\n")
		})
	})
	t.When("Equal fails on values whose GoString methods render them alike", func(w *gotest.T) {
		w.It("the message shows the difference of their raw fields", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, masked{1, 2}, masked{1, 3}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "the values render alike; their raw fields differ:\n  expected: assertions_test.masked{Shown:1, hidden:2}\n  actual:   assertions_test.masked{Shown:1, hidden:3}")
		})
	})
	t.When("Equal fails on values that render alike, such as two NaNs beside distinct pointers to equal values", func(w *gotest.T) {
		w.It("the message names where they differ, says the rendering does not show it and shows the value once, without addresses", func(it *gotest.T) {
			x, y := 1, 1
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, measure{&x, math.NaN()}, measure{&y, math.NaN()}) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "values differ at .F in ways their rendering does not show")
			gotest.Equal(it, 1, strings.Count(m.Message(), "assertions_test.measure{P:&int(1), F:NaN}"))
			gotest.NotContains(it, m.Message(), "0x")
		})
	})
	t.When("NotEqual fails on values holding pointers", func(w *gotest.T) {
		w.It("the message shows the value they share without addresses", func(it *gotest.T) {
			v := item{"a", &owner{"o"}}
			m := gotest.Record(func(r *gotest.R) { gotest.NotEqual(r, v, v) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), `both are: assertions_test.item{Name:"a", Owner:&assertions_test.owner{Name:"o"}}`)
			gotest.NotContains(it, m.Message(), "0x")
		})
	})
}

// masked's GoString leaves out a field.
type masked struct{ Shown, hidden int }

func (m masked) GoString() string { return "masked(" + strconv.Itoa(m.Shown) + ")" }

type measure struct {
	P *int
	F float64
}
