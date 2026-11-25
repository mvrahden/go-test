// Command label prints calc's label of its argument.
package main

import (
	"fmt"
	"os"
	"strconv"

	"mutmod/calc"
)

func main() {
	n, err := strconv.Atoi(os.Args[1])
	if err != nil {
		os.Exit(2)
	}
	fmt.Println(calc.Label(n))
}
