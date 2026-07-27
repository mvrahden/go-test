package fuzzcodec

type Priority int

type Label string

type Address struct {
	Street string
	Zip    uint16
}

// Rich exercises every kind the Phase A wire format supports: basics of
// every width, named types over basics, string and []byte, a slice, a fixed
// array, a pointer, and a nested struct — inline and behind a slice.
type Rich struct {
	Name     string
	Blob     []byte
	OK       bool
	I        int
	I8       int8
	I16      int16
	I32      int32
	I64      int64
	U        uint
	U8       uint8
	U16      uint16
	U32      uint32
	U64      uint64
	F32      float32
	F64      float64
	Prio     Priority
	Tag      Label
	Tags     []string
	Grid     [3]int8
	Home     *Address
	Nested   Address
	Counters []Address
}
