package store_test

import (
	"time"

	"example.com/shopd/store"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreTestSuite struct{}

func (s *StoreTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 30 * time.Second, SetupTimeout: 30 * time.Second}
}

func (s *StoreTestSuite) TestReserve1(t *gotest.T) {
	inv := store.NewInventory()
	inv.Add("apple", 10)
	inv.Add("pear", 5)
	gotest.NoError(t, inv.Reserve("apple", 3))
	gotest.Equal(t, 7, inv.Stock("apple"))
}

func (s *StoreTestSuite) TestReserve2(t *gotest.T) {
	inv := store.NewInventory()
	inv.Add("apple", 10)
	inv.Add("pear", 5)
	gotest.Error(t, inv.Reserve("apple", 11))
	gotest.Equal(t, 10, inv.Stock("apple"))
}

func (s *StoreTestSuite) TestReserve3(t *gotest.T) {
	inv := store.NewInventory()
	inv.Add("apple", 10)
	inv.Add("pear", 5)
	gotest.NoError(t, inv.Reserve("pear", 5))
	gotest.Equal(t, 0, inv.Stock("pear"))
}

func (s *StoreTestSuite) TestSnapshot(t *gotest.T) {
	dir := t.TempDir()
	ss, err := store.OpenSnapshotStore(dir)
	gotest.NoError(t, err)
	defer ss.Close()
	inv := store.NewInventory()
	inv.Add("apple", 2)
	gotest.NoError(t, ss.Save("snap", inv))
	got, err := ss.Load("snap")
	gotest.NoError(t, err)
	gotest.Equal(t, 2, got["apple"])
}

func (s *StoreTestSuite) TestRestock(t *gotest.T) {
	inv := store.NewInventory()
	inv.Add("apple", 1)
	r := store.NewRestocker(inv, 10*time.Millisecond)
	r.Start("apple", 5)
	time.Sleep(300 * time.Millisecond)
	r.Stop()
	gotest.Equal(t, 5, inv.Stock("apple"))
}
