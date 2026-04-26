package http3stub

import (
	"encoding/binary"
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestRFC9484PeelContextZero(t *testing.T) {
	var inner []byte
	inner = append(inner, 0x45, 0x00)
	inner = binary.BigEndian.AppendUint16(inner, 20)
	inner = append(inner, 0x00, 0x00, 0x00, 0x00, 64, 17, 0x00, 0x00)
	inner = append(inner, 10, 0, 0, 1)
	inner = append(inner, 198, 51, 100, 9)

	wrapped := encodeRFC9484ContextZeroIPPacket(inner)
	payload, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(wrapped, nil)
	if drop || unk {
		t.Fatalf("drop=%v unk=%v", drop, unk)
	}
	if string(payload) != string(inner) {
		t.Fatalf("payload len=%d want %d", len(payload), len(inner))
	}
}

func TestRFC9484PeelUnknownContext(t *testing.T) {
	b := quicvarint.Append(nil, 42)
	b = append(b, 0x45, 0x00, 0x00)
	_, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(b, nil)
	if !drop || !unk {
		t.Fatalf("drop=%v unk=%v", drop, unk)
	}
}

func TestRFC9484PeelInvalidVarintFallsBack(t *testing.T) {
	// Single 0xff byte: 8-byte varint encoding but truncated -> Parse error -> full slice
	b := []byte{0xff}
	payload, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(b, nil)
	if drop || unk || len(payload) != 1 || payload[0] != 0xff {
		t.Fatalf("payload=%v drop=%v unk=%v", payload, drop, unk)
	}
}

func TestParseIPv4WrappedRFC9484(t *testing.T) {
	var inner []byte
	inner = append(inner, 0x45, 0x00)
	inner = binary.BigEndian.AppendUint16(inner, 28)
	inner = append(inner, 0x00, 0x00, 0x00, 0x00, 64, 17, 0x00, 0x00)
	inner = append(inner, 10, 0, 0, 1)
	inner = append(inner, 198, 51, 100, 11)
	inner = append(inner, 0x12, 0x34, 0x00, 0x35)
	inner = binary.BigEndian.AppendUint16(inner, 8)
	inner = append(inner, 0x00, 0x00)

	wrapped := encodeRFC9484ContextZeroIPPacket(inner)
	payload, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(wrapped, nil)
	if drop || unk {
		t.Fatal("peel")
	}
	dst, proto, port, ok := parseConnectIPDatagramDestination(payload)
	if !ok || proto != "udp" || port != 53 {
		t.Fatalf("ok=%v proto=%q port=%d", ok, proto, port)
	}
	if dst.String() != "198.51.100.11" {
		t.Fatalf("dst=%v", dst)
	}
}

func TestRFC9484PeelAllowlistedContext(t *testing.T) {
	allow := map[uint64]struct{}{2: {}}
	b := quicvarint.Append(nil, 2)
	b = append(b, 0x01, 0x02, 0x03)
	payload, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(b, allow)
	if drop || unk {
		t.Fatalf("drop=%v unk=%v", drop, unk)
	}
	if string(payload) != "\x01\x02\x03" {
		t.Fatalf("payload=%q", payload)
	}
}

func TestRFC9484PeelAllowlistedContextEmptyInner(t *testing.T) {
	allow := map[uint64]struct{}{4: {}}
	b := quicvarint.Append(nil, 4)
	_, drop, unk := rfc9484ConnectIPDatagramPayloadForPolicy(b, allow)
	if !drop || unk {
		t.Fatalf("drop=%v unk=%v", drop, unk)
	}
}
