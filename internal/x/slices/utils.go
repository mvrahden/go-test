package slices

import "sort"

// FindIndex will search with given test func.
// When the test is positive it will return its index and `-1` otherwise.
func FindIndex[T any](in []T, fn func(v T, idx int) bool) int {
	for idx, v := range in {
		ok := fn(v, idx)
		if ok {
			return idx
		}
	}
	return -1
}

// Filter will apply a filter operation on the input.
func Filter[T any](in []T, fn func(v T, idx int) bool) []T {
	var out []T
	for idx, v := range in {
		ok := fn(v, idx)
		if !ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

// Count will apply a count operation on the input.
func Count[T any](in []T, fn func(v T, idx int) bool) int {
	var out int
	for idx, v := range in {
		ok := fn(v, idx)
		if !ok {
			continue
		}
		out++
	}
	return out
}

// Map will apply a map operation on the input.
func Map[T, R any](in []T, fn func(v T, idx int) R) []R {
	out := make([]R, len(in))
	for idx, v := range in {
		out[idx] = fn(v, idx)
	}
	return out
}

// Reduce will apply a reduce operation on the input.
// The seed value of the accumulator will be its zero value.
func Reduce[T, R any](in []T, fn func(v T, acc R) R) R {
	var out R
	for _, v := range in {
		out = fn(v, out)
	}
	return out
}

// ReduceSeed will apply a reduce operation on the input with
// given seed value for the accumulator.
func ReduceSeed[T, R any](in []T, seed R, fn func(v T, acc R) R) R {
	for _, v := range in {
		seed = fn(v, seed)
	}
	return seed
}

// Range will range over given input and apply the given callback func on each element.
func Range[T any](in []T, fn func(v T, idx int)) {
	for idx, v := range in {
		fn(v, idx)
	}
}

// RangeErr will range over given input and apply the given callback func on each element.
// It will fail on first error, returning the emitted error and the index which failed.
// If no error was emitted the returned index will be `-1`.
func RangeErr[T any](in []T, fn func(v T, idx int) error) (int, error) {
	for idx, v := range in {
		err := fn(v, idx)
		if err != nil {
			return idx, err
		}
	}
	return -1, nil
}

// SplitBy will range over given input and apply the given test func on each element.
// It will collect all positive results to the first bucket and all negative results in the second.
func SplitBy[T any](in []T, fn func(v T, idx int) bool) (t []T, f []T) {
	for idx, v := range in {
		ok := fn(v, idx)
		if ok {
			t = append(t, v)
			continue
		}
		f = append(f, v)
	}
	return t, f
}

// None will apply given test func on each input and assert
// that none of the inputs passes the test.
// It returns on first failed assertion.
func None[T any](in []T, fn func(v T, idx int) bool) bool {
	ok := Any(in, fn)
	return !ok
}

// Any will apply given test func on each input and assert
// that any of the inputs passes the test.
// It returns on first succeeded assertion.
func Any[T any](in []T, fn func(v T, idx int) bool) bool {
	for idx, v := range in {
		ok := fn(v, idx)
		if ok {
			return true
		}
	}
	return false
}

// All will apply given test func on each input and assert
// that all of the inputs pass the test.
// It return on first failed assertion.
func All[T any](in []T, fn func(v T, idx int) bool) bool {
	for idx, v := range in {
		ok := fn(v, idx)
		if !ok {
			return false
		}
	}
	return true
}

// Copy will return a shallow copy of the input.
func Copy[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// Sort will sort a shallow copy of the given input based on given
// "less" func, leaving the input untouched.
// Within the "less" func you have to use the injected reference of the slice.
func Sort[T any](in []T, less func(s []T, i, j int) bool) []T {
	out := Copy(in)
	sort.Slice(out, func(i, j int) bool {
		return less(out, i, j)
	})
	return out
}

// Sort will sort a shallow copy of the given input based on given
// "less" func, leaving the input untouched.
// Within the "less" func you have to use the injected reference of the slice.
func SortStable[T any](in []T, less func(s []T, i, j int) bool) []T {
	out := Copy(in)
	sort.Slice(out, func(i, j int) bool {
		return less(out, i, j)
	})
	return out
}
