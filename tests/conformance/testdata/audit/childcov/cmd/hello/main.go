// Command hello prints greet's greeting of its argument.
package main

import (
	"fmt"
	"os"

	"childcov/greet"
)

func main() {
	fmt.Println(greet.Hello(os.Args[1]))
}
