package capsules9297

import (
	"bytes"
	"errors"
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestDrainOneCapsule(t *testing.T) {
	var wire []byte
	wire = quicvarint.Append(wire, 7)     // type
	wire = quicvarint.Append(wire, 3)     // length
	wire = append(wire, []byte("abc")...) // value

	var st Stats
	if err := Drain(bytes.NewReader(wire), DrainOptions{MaxWireBytes: 100, MaxCapsuleBody: 100}, &st); err != nil {
		t.Fatal(err)
	}
	if st.Capsules != 1 {
		t.Fatalf("capsules=%d", st.Capsules)
	}
	if st.ByType[7] != 1 {
		t.Fatalf("byType=%v", st.ByType)
	}
}

func TestDrainEmptyStream(t *testing.T) {
	var st Stats
	if err := Drain(bytes.NewReader(nil), DrainOptions{MaxWireBytes: 10}, &st); err != nil {
		t.Fatal(err)
	}
	if st.Capsules != 0 {
		t.Fatal(st.Capsules)
	}
}

func TestDrainTrailingGarbage(t *testing.T) {
	wire := []byte{0x01} // incomplete varint
	var st Stats
	err := Drain(bytes.NewReader(wire), DrainOptions{MaxWireBytes: 100}, &st)
	if err == nil || !errors.Is(err, ErrTrailingBytes) {
		t.Fatalf("err=%v", err)
	}
}

func TestDrainTooLarge(t *testing.T) {
	var wire []byte
	wire = quicvarint.Append(wire, 1)
	wire = quicvarint.Append(wire, 9999)
	wire = append(wire, make([]byte, 9999)...)

	var st Stats
	err := Drain(bytes.NewReader(wire), DrainOptions{MaxWireBytes: 1 << 20, MaxCapsuleBody: 100}, &st)
	if !errors.Is(err, ErrCapsuleTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestDrainWireLimit(t *testing.T) {
	r := bytes.NewReader(make([]byte, 100))
	var st Stats
	err := Drain(r, DrainOptions{MaxWireBytes: 40, MaxCapsuleBody: 100}, &st)
	if !errors.Is(err, ErrWireLimit) {
		t.Fatalf("err=%v", err)
	}
}

func TestDrainPerCapsuleAbort(t *testing.T) {
	var wire []byte
	wire = quicvarint.Append(wire, 7)
	wire = quicvarint.Append(wire, 0)

	var st Stats
	err := Drain(bytes.NewReader(wire), DrainOptions{
		MaxWireBytes:   100,
		MaxCapsuleBody: 100,
		PerCapsule: func(typ uint64, payload []byte) error {
			if typ == 7 {
				return errors.New("stop")
			}
			return nil
		},
	}, &st)
	if err == nil || err.Error() != "stop" {
		t.Fatalf("err=%v", err)
	}
}
