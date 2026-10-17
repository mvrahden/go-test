package withsimplify //nolint:stdlib-test

import (
	errs "errors"
	refl "reflect"
	str "strings"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// === Calls into packages imported under another name ===

// The fixes leave the imports in use.
var _, _ = str.ToUpper, refl.TypeOf

func TestAliasedPackages(t *testing.T) {
	gotest.True(t, str.Contains("abc", "b"))              // want `use Contains instead of True for strings.Contains call`
	gotest.True(t, errs.Is(errs.New("x"), errs.New("y"))) // want `use ErrorIs instead of True for errors.Is call`
	gotest.True(t, refl.DeepEqual(1, 2))                  // want `use Equal instead of True for reflect.DeepEqual call`
}

// === Lookalikes that are not those packages ===

type set struct{}

func (set) Contains(a, b string) bool { return a == b }
func (set) Is(a, b error) bool        { return a == b }

func TestLookalikes(t *testing.T) {
	strings, errors := set{}, set{}
	gotest.True(t, strings.Contains("a", "b"))
	gotest.True(t, errors.Is(nil, nil))
}
