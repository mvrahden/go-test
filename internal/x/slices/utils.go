package slices

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

// MapErr will apply a map operation on the input and will terminate on first error.
func MapErr[T, R any](in []T, fn func(v T, idx int) (R, error)) ([]R, error) {
	out := make([]R, len(in))
	for idx, v := range in {
		var err error
		out[idx], err = fn(v, idx)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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
