// Command stdinecho prints the first line of its standard input.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if line == "" && err != nil {
		os.Exit(1)
	}
	fmt.Println(strings.TrimRight(line, "\r\n"))
}
