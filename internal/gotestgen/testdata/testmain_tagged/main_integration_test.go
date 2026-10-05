//go:build integration

package testmaintagged

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

func TestMain(m *testing.M) { os.Exit(gotestruntime.Main(m)) }
