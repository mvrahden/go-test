package store

import (
	"context"
	"os"

	"example.com/runtimecompat/fixtures"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct {
	Shared *fixtures.StoreSharedFixture
}

func (f *DBFixture) FixtureConfig() gotest.FixtureConfig { return gotest.DefaultFixtureConfig() }

func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *DBFixture) AfterAll(ctx context.Context) error { return nil }

func (f *DBFixture) BeforeEach(ctx context.Context) error { return nil }

func (f *DBFixture) AfterEach(ctx context.Context) error { return nil }

type StoreTestSuite struct {
	DB *DBFixture
}

type storeCtx struct {
	keys []string
}

func (s *StoreTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *StoreTestSuite) SuiteGuard() string {
	if os.Getenv("RUNTIMECOMPAT_SKIP") != "" {
		return "RUNTIMECOMPAT_SKIP set"
	}
	return ""
}

func (s *StoreTestSuite) BeforeAll(t *gotest.T) {}

func (s *StoreTestSuite) AfterAll(t *gotest.T) {}

func (s *StoreTestSuite) BeforeEach(t *gotest.T) *storeCtx {
	return &storeCtx{keys: []string{" A ", "b"}}
}

func (s *StoreTestSuite) AfterEach(t *gotest.T, c *storeCtx) {}

func (s *StoreTestSuite) TestSharedAddr(t *gotest.T, c *storeCtx) {
	gotest.Equal(t, "mem://store", s.DB.Shared.Addr)
	gotest.NotZero(t, s.DB.Shared.Log)
}

func (s *StoreTestSuite) TestNormalize(t *gotest.T, c *storeCtx) {
	rows := []struct {
		Name string
		In   string
		Want string
	}{
		{Name: "trims", In: " A ", Want: "a"},
		{Name: "keeps", In: "b", Want: "b"},
	}
	for t, row := range gotest.Each(t, rows) {
		gotest.Equal(t, row.Want, Normalize(row.In))
	}
	t.When("keys come from BeforeEach", func(w *gotest.T) {
		gotest.Len(w, c.keys, 2)
	})
}

type CodecTestSuite struct {
	seen int
}

func (s *CodecTestSuite) BeforeEach(t *gotest.T) { s.seen = 0 }

func (s *CodecTestSuite) AfterEach(t *gotest.T) {}

func (s *CodecTestSuite) TestIdempotent(t *gotest.T) {
	gotest.Equal(t, "a", Normalize(Normalize(" A ")))
}

func (s *CodecTestSuite) BenchmarkNormalize(b *gotest.B) {
	for b.Loop() {
		Normalize(" A ")
	}
}

func (s *CodecTestSuite) FuzzNormalize(f *gotest.F) {
	f.Add(" A ")
	f.Fuzz(func(t *gotest.T, key string) {
		once := Normalize(key)
		gotest.Equal(t, once, Normalize(once))
	})
}
