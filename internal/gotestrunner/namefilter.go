package gotestrunner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// nameFilter is a -run, -skip or -bench pattern read as the testing package
// reads it (testing/match.go): alternatives split on '|', each a sequence of
// per-level patterns split on '/', neither inside brackets or parentheses.
type nameFilter struct {
	alts [][]*regexp.Regexp
}

// parseNameFilter compiles pattern as the test binary would; flagName (e.g.
// "-skip") names it in the error, which spells the failing part as testing
// does.
func parseNameFilter(pattern, flagName string) (nameFilter, error) {
	alts, alternation := splitNameFilter(pattern)
	f := nameFilter{alts: make([][]*regexp.Regexp, len(alts))}
	for i, elems := range alts {
		f.alts[i] = make([]*regexp.Regexp, len(elems))
		for j, e := range elems {
			e = rewriteName(e)
			re, err := regexp.Compile(e)
			if err != nil {
				err = fmt.Errorf("element %d of %s (%q): %s", j, flagName, e, err)
				if alternation {
					err = fmt.Errorf("alternation %d of %w", i, err)
				}
				return nameFilter{}, err
			}
			f.alts[i][j] = re
		}
	}
	return f, nil
}

// matchTop matches a top-level function name: the first alternative whose
// first element matches decides, and the match is partial when that
// alternative goes on to subtest levels.
func (f nameFilter) matchTop(name string) (ok, partial bool) {
	for _, elems := range f.alts {
		if elems[0].MatchString(name) {
			return true, len(elems) > 1
		}
	}
	return false, false
}

// splitNameFilter is testing's splitRegexp: alternatives on '|', each split
// into levels on '/'. alternation reports whether there was a '|'.
func splitNameFilter(s string) (alts [][]string, alternation bool) {
	for _, alt := range splitTopLevel(s, '|') {
		alts = append(alts, splitTopLevel(alt, '/'))
	}
	return alts, len(alts) > 1
}

// splitTopLevel cuts s at each sep outside brackets and parentheses, counted
// as testing's splitRegexp counts them: a backslash escapes the next byte,
// an unmatched ']' is literal, and parentheses inside brackets do not count.
// Cutting on '|' first and on '/' after yields testing's elements, since
// both cuts happen only where both counts are zero.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	cs, cp := 0, 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			cs++
		case ']':
			if cs--; cs < 0 {
				cs = 0
			}
		case '(':
			if cs == 0 {
				cp++
			}
		case ')':
			if cs == 0 {
				cp--
			}
		case '\\':
			i++
		case sep:
			if cs == 0 && cp == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// rewriteName is testing's rewrite: white space becomes '_' and a
// non-printable rune its quoted escape.
func rewriteName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case isNameSpace(r):
			b.WriteByte('_')
		case !strconv.IsPrint(r):
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isNameSpace(r rune) bool {
	if r < 0x2000 {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0, 0x1680:
			return true
		}
		return false
	}
	if r <= 0x200a {
		return true
	}
	switch r {
	case 0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return false
}

// selectsFunc reports whether the -run filter enters a top-level function. A
// filter that does not compile selects everything: the binary reports it.
func selectsFunc(userRunFilter, funcName string) bool {
	if userRunFilter == "" {
		return true
	}
	f, err := parseNameFilter(userRunFilter, "-run")
	if err != nil {
		return true
	}
	ok, _ := f.matchTop(funcName)
	return ok
}

// skipsFunc reports whether the -skip filter keeps a top-level function from
// running at all: the deciding alternative matches it with no subtest level
// left over. A partial match only skips subtests inside it, and a filter that
// does not compile skips nothing: the binary reports it.
func skipsFunc(userSkipFilter, funcName string) bool {
	if userSkipFilter == "" {
		return false
	}
	f, err := parseNameFilter(userSkipFilter, "-skip")
	if err != nil {
		return false
	}
	ok, partial := f.matchTop(funcName)
	return ok && !partial
}

// selectsBench reports whether a bench run dispatches the benchmark wrapper
// benchFuncName: -run and -bench each enter it (gotest bench reads -run as a
// suite filter too), and -skip does not skip it, as testing decides for
// benchmarks.
func selectsBench(userRunFilter, userBenchFilter, userSkipFilter, benchFuncName string) bool {
	return selectsFunc(userRunFilter, benchFuncName) && selectsFunc(userBenchFilter, benchFuncName) && !skipsFunc(userSkipFilter, benchFuncName)
}
