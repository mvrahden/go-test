package parallelsuite

import "sync/atomic"

var counter atomic.Int64

func Increment() int64 {
	return counter.Add(1)
}
