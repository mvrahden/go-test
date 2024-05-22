package main

import (
	"log"
	"os"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func main() {
	err := testgen.Execute(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}
