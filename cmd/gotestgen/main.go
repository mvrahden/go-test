package gotestgen

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func main() {
	err := Generate()
	if err != nil {
		fmt.Printf("failed generating test harness: %s\n", err)
		os.Exit(1)
	}
}

func Generate() error {
	return gotestgen.Generate()
}
