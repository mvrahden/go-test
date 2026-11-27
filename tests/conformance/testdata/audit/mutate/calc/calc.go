// Package calc holds the lines audit --mutate judges; each is changed.
package calc

// Grade names a score: pass from 50 up. Only report's tests call it.
func Grade(score int) string {
	if score >= 50 {
		return "pass"
	}
	return "fail"
}

// Discount is a tenth off a total over 100. Its test never gives 100.
func Discount(total int) int {
	if total > 100 {
		return total / 10
	}
	return 0
}

// Sum adds n, n-1, …, 1.
func Sum(n int) int {
	s := 0
	for i := n; i > 0; { //gotest:mutate-ignore at i == 0 the loop adds nothing
		s += i
		i--
	}
	return s
}

// Unused is called by no test.
func Unused(x int) bool {
	return x == 0
}

//gotest:mutate-ignore
func Half(n int) int {
	return n / 2
}

// Label names a count; only the label binary calls it.
func Label(n int) string {
	if n == 1 {
		return "one"
	}
	return "many"
}
