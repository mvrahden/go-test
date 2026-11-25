// Command plain prints plain's goodbye to its argument.
package main

import (
	"fmt"
	"os"

	"childcov/plain"
)

func main() {
	fmt.Println(plain.Bye(os.Args[1]))
}
