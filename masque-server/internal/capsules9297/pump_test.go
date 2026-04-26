package capsules9297

import (
	"bytes"
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestPumpWritesReply(t *testing.T) {
	var wire []byte
	wire = quicvarint.Append(wire, 9)
	wire = quicvarint.Append(wire, 0)

	var out bytes.Buffer
	var st Stats
	err := Pump(bytes.NewReader(wire), &out, DrainOptions{MaxWireBytes: 100, MaxCapsuleBody: 100}, &st,
		func(typ uint64, payload []byte) ([][]byte, error) {
			if typ != 9 {
				return nil, nil
			}
			return [][]byte{[]byte("ACK")}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "ACK" {
		t.Fatalf("got %q", out.String())
	}
}
