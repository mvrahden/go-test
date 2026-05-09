package assert

import (
	"reflect"
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

var (
	rwMutexType     = reflect.TypeFor[sync.RWMutex]()
	helperPCsType   = reflect.TypeFor[map[uintptr]struct{}]()
	helperNamesType = reflect.TypeFor[map[string]struct{}]()
)

// SkipInternalFrames walks the call stack and registers all gotest-internal
// frame PCs in tt's helperPCs map. This causes Go's testing.T.decorate to
// skip those frames when choosing the file:line prefix for error output.
//
// Uses reflect + unsafe to access unexported fields. Runtime type guards
// ensure a silent no-op if Go's testing internals change.
func SkipInternalFrames(tt *testing.T) {
	if tt == nil {
		return
	}

	v := reflect.ValueOf(tt).Elem()

	muField := v.FieldByName("mu")
	hpField := v.FieldByName("helperPCs")
	hnField := v.FieldByName("helperNames")

	if !muField.IsValid() || !muField.CanAddr() || muField.Type() != rwMutexType {
		return
	}
	if !hpField.IsValid() || !hpField.CanAddr() || hpField.Type() != helperPCsType {
		return
	}

	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs) // skip runtime.Callers + SkipInternalFrames
	if n == 0 {
		return
	}

	var toMark []uintptr
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if IsGotestSource(frame.File) || IsGeneratedBridge(frame.File) {
			toMark = append(toMark, frame.PC)
		}
		if !more {
			break
		}
	}

	if len(toMark) == 0 {
		return
	}

	mu := (*sync.RWMutex)(unsafe.Pointer(muField.UnsafeAddr()))
	mu.Lock()
	defer mu.Unlock()

	hp := (*map[uintptr]struct{})(unsafe.Pointer(hpField.UnsafeAddr()))
	if *hp == nil {
		*hp = make(map[uintptr]struct{})
	}
	for _, pc := range toMark {
		(*hp)[pc] = struct{}{}
	}

	if hnField.IsValid() && hnField.CanAddr() && hnField.Type() == helperNamesType {
		hn := (*map[string]struct{})(unsafe.Pointer(hnField.UnsafeAddr()))
		*hn = nil
	}
}
