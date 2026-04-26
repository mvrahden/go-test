package gotest

import "fmt"

func Must[T any](val T, ok any) T {
	switch v := ok.(type) {
	case nil:
		return val
	case bool:
		if v {
			return val
		}
		panic("Must: got false")
	case error:
		panic(fmt.Sprintf("Must: got error: %v", v))
	default:
		return val
	}
}
