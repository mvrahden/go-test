package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"example.com/shopd/store"
)

func TestTotal(t *testing.T) {
	c := store.StaticCatalog{"apple": 100, "pear": 250}
	got, err := Total(c, []string{"apple", "apple", "pear"})
	require.NoError(t, err)
	require.Equal(t, 450, got)
}

func TestTotalEmpty(t *testing.T) {
	got, err := Total(store.StaticCatalog{}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, got)
}

func TestTotalUnknownSKU(t *testing.T) {
	c := store.StaticCatalog{"apple": 100}
	_, err := Total(c, []string{"banana"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "banana")
}
