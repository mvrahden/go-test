//go:build fixsbuildflags

package fixsbuildflags

import "context"

// TaggedSharedFixture exists only under the tag.
type TaggedSharedFixture struct {
	Addr string
}

func (f *TaggedSharedFixture) BeforeAll(ctx context.Context) error { f.Addr = "up"; return nil }
