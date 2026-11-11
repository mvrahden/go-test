package gotestrunner

import (
	"bytes"
	"encoding/json"

	"github.com/mvrahden/go-test/internal/protocol"
)

// stampBlock adds the Gotest object to the test events of one suite
// process's -json block; every other byte is kept. The block is the
// process's whole stream in its real order, which is what places a test.
// A line that is no test event, or has nothing to stamp, is copied as is.
// Each package is placed apart: a block the runner writes itself may hold
// several.
func stampBlock(block []byte) []byte {
	placers := map[string]*protocol.Placer{}
	out := make([]byte, 0, len(block)+len(block)/8)
	for len(block) > 0 {
		line := block
		if i := bytes.IndexByte(block, '\n'); i >= 0 {
			line = block[:i+1]
		}
		block = block[len(line):]
		out = append(out, stampLine(placers, line)...)
	}
	return out
}

// stampLine splices ,"Gotest":{…} before the line's final }.
func stampLine(placers map[string]*protocol.Placer, line []byte) []byte {
	var ev struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Test    string `json:"Test"`
	}
	body := bytes.TrimRight(line, " \t\r\n")
	if !bytes.HasSuffix(body, []byte("}")) || json.Unmarshal(body, &ev) != nil || ev.Test == "" {
		return line
	}
	placer := placers[ev.Package]
	if placer == nil {
		placer = protocol.NewPlacer()
		placers[ev.Package] = placer
	}
	st := protocol.StampOf(ev.Action, placer.Place(ev.Action, ev.Test))
	if st == (protocol.Stamp{}) {
		return line
	}
	obj, err := json.Marshal(st)
	if err != nil {
		return line
	}
	end := len(body) - 1
	stamped := make([]byte, 0, len(line)+len(obj)+10)
	stamped = append(stamped, line[:end]...)
	stamped = append(stamped, `,"Gotest":`...)
	stamped = append(stamped, obj...)
	return append(stamped, line[end:]...)
}
