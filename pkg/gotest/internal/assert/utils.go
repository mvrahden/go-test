package assert

import (
	"reflect"
	"strings"
)

const (
	fmtToken = `{{FMT}}`
)

func applyFmtTokens(fmt string, v ...any) string {
	for _, v := range v {
		fmt = applyNextFmtToken(fmt, v)
	}
	return fmt
}

func applyNextFmtToken(fmt string, v any) string {
	idx := strings.Index(fmt, fmtToken)
	if idx == -1 {
		return fmt
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func:
		return fmt[:idx] + "Type<%T>" + fmt[idx+len(fmtToken):]
	}
	return fmt[:idx] + "%v" + fmt[idx+len(fmtToken):]
}
func unwrapPtr(r reflect.Value) reflect.Value {
	k := r.Kind()
	if k != reflect.Pointer && k != reflect.Interface {
		return r
	}
	if !r.IsNil() {
		return unwrapPtr(r.Elem())
	}
	return r
}
