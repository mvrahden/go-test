package fuzzing

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type FrameCodecTestSuite struct {
	codec *Codec
}

// BeforeEach rebuilds the codec before every execution — not just before
// every top-level test. Fuzz targets replay this hook around each fuzz
// execution too, so a codec left over from a previous execution never
// leaks state into the next one.
func (s *FrameCodecTestSuite) BeforeEach(t *gotest.T) {
	s.codec = NewCodec(1 << 20) // a 1 MiB broker-configured payload limit
}

// TestNormalizeTopicTable exercises the same NormalizeTopic calls
// FuzzNormalizeTopicIdempotent's callback invokes — gotest generate
// harvests its literal table rows as extra f.Add(...) seeds for
// FuzzFrameCodecTestSuite_FuzzNormalizeTopicIdempotent.
func (s *FrameCodecTestSuite) TestNormalizeTopicTable(t *gotest.T) {
	type tc struct {
		Desc string
		In   string
		Want string
	}
	for t, c := range gotest.Each(t, []tc{
		{"already normalized", "orders.created", "orders.created"},
		{"mixed case", "Orders.Created", "orders.created"},
		{"repeated separators", "orders..created", "orders.created"},
		{"surrounding whitespace and dots", "  .orders.created.  ", "orders.created"},
	}) {
		t.It("normalizes to the expected result", func(t *gotest.T) {
			gotest.Equal(t, c.Want, NormalizeTopic(c.In))
		})
	}
}

func (s *FrameCodecTestSuite) TestEncodeDecodeExamples(t *gotest.T) {
	t.When("a minimal frame with no headers, payload, or trace", func(t *gotest.T) {
		in := Frame{Version: 1, Kind: KindData, Topic: "orders.created"}
		out, err := Decode(Encode(in))

		t.It("round-trips exactly", func(t *gotest.T) {
			gotest.NoError(t, err)
			gotest.Equal(t, in, out)
		})
	})

	t.When("a frame with headers, a payload, and a trace ID", func(t *gotest.T) {
		in := Frame{
			Version: 2,
			Kind:    KindControl,
			Topic:   "orders.shipped",
			Headers: []Header{{Name: "content-type", Value: "application/json"}},
			Payload: []byte(`{"id":42}`),
			Trace:   &TraceID{Hi: 0xC0FFEE, Lo: 0x1234},
		}
		out, err := Decode(Encode(in))

		t.It("round-trips exactly", func(t *gotest.T) {
			gotest.NoError(t, err)
			gotest.Equal(t, in, out)
		})
	})
}

// FuzzFrameRoundTrip is the flagship target: Frame is not one of the
// fifteen types Go's fuzzing engine accepts, so gotest generates a codec
// for it and reroutes the target to a native []byte one. The seed below is
// a plain Go literal — F.Add encodes it on the way in.
func (s *FrameCodecTestSuite) FuzzFrameRoundTrip(f *gotest.F) {
	f.Add(Frame{
		Version: 1,
		Kind:    KindControl,
		Topic:   "orders.created",
		Headers: []Header{{Name: "content-type", Value: "application/json"}},
		Payload: []byte(`{"id":1}`),
		Trace:   &TraceID{Hi: 1, Lo: 2},
	})
	// Regression seed for the packed-Version/Kind byte bug: `gotest fuzz`
	// found that Version: 48 alone (>= 32) overflowed the 5 bits it shared
	// with Kind in the single packed byte, corrupting the decoded Version.
	// Promoted by `gotest fuzz promote` from the crasher it produced.
	f.Add(Frame{Version: 48, Kind: Kind(0), Topic: "", Headers: nil, Payload: nil, Trace: nil})
	gotest.Fuzz(f, func(t *gotest.T, in Frame) {
		out, err := Decode(Encode(in))
		gotest.NoError(t, err)
		gotest.Equal(t, in, out) // property: round trip
	})
}

// FuzzDecodeNeverPanics is a native []byte target over adversarial input:
// decoding must never panic, and anything that decodes must re-encode and
// decode again to the very same value — a decoder that accepts input it
// cannot round-trip is lying about success. That re-decode stability is a
// real oracle, not just crash-detection: it fails on a decoder that is
// merely panic-free but still silently wrong.
func (s *FrameCodecTestSuite) FuzzDecodeNeverPanics(f *gotest.F) {
	f.Add([]byte{})
	f.Add([]byte("not a frame at all"))
	gotest.Fuzz(f, func(t *gotest.T, in []byte) {
		out, err := s.codec.Decode(in)
		if err != nil {
			return // rejecting malformed or oversized input is correct behaviour, not a failure
		}
		again, err := s.codec.Decode(Encode(out))
		gotest.NoError(t, err)
		gotest.Equal(t, out, again)
	})
}

// FuzzNormalizeTopicIdempotent is a native string target: idempotence.
func (s *FrameCodecTestSuite) FuzzNormalizeTopicIdempotent(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, topic string) {
		once := NormalizeTopic(topic)
		gotest.Equal(t, once, NormalizeTopic(once)) // property: idempotence
	})
}

// FuzzTopicMatches — multi-argument fuzz targets take only Go's native
// fuzzable types; structured input (like Frame above) belongs in a single
// struct-typed target instead, not spread across Fuzz2/Fuzz3 arguments.
func (s *FrameCodecTestSuite) FuzzTopicMatches(f *gotest.F) {
	f.Add("orders.created", "ORDERS.CREATED")
	gotest.Fuzz2(f, func(t *gotest.T, a, b string) {
		// Property: matching is symmetric, whatever the input.
		gotest.Equal(t, TopicMatches(a, b), TopicMatches(b, a))
	})
}
