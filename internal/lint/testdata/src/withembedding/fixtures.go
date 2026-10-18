package withembedding

import "context"

type base struct{}

func (b *base) AfterAll(context.Context) error { return nil }

type QueueSharedFixture struct {
	base // want `fixture QueueSharedFixture embeds base, whose method AfterAll gotest would read; keep helpers in a named field or in functions`
}

func (f *QueueSharedFixture) BeforeAll(context.Context) error { return nil }
