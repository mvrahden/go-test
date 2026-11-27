package refused

import "time"

const limit = 1

type celsius int

var grid = [limit + 1]int{1: 5}

func Warm(c celsius) bool {
start:
	for c < 20 {
		c++
		continue start
	}
	return c == celsius(limit)
}

func Pause() time.Duration {
	return time.Duration(1) * time.Millisecond
}

func Span[T ~int](n T) T {
	var zero T = 0
	total := zero
	for i := T(1); i < n; i++ {
		total += i
	}
	return total
}

func Long(a, b int) bool {
	return a <
		b
}

func Fail() {
	panic("refused")
}
