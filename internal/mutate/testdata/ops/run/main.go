package main

import "fmt"

const limit = 3

type level int

type box struct{ n int }

func (b *box) add(v int) {
	b.n = b.n + v
}

func maxOf[T int | float64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func main() {
	var arr [limit]int
	b := &box{}
	b.add(1)
	ok := b.n == 1 && len(arr) == limit
	double := func(x int) int { return x + 10 }
	for i := 0; i < 2; i++ {
		b.add(double(i))
	}
	var lv level = 1
	fmt.Println(b.n, ok, maxOf(2, 5), maxOf(1.5, 0.5), lv, true)
}
