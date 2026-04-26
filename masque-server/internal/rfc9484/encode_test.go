package rfc9484

import (
	"net"
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestEncodeDecodeAddressAssignRoundTrip(t *testing.T) {
	addrs := []AssignedAddress{
		{RequestID: 7, IPVersion: 4, IP: net.ParseIP("192.0.2.9").To4(), PrefixLength: 32},
	}
	wire, err := EncodeAddressAssign(addrs)
	if err != nil {
		t.Fatal(err)
	}
	typ, n1, err := quicvarint.Parse(wire)
	if err != nil {
		t.Fatal(err)
	}
	if typ != CapsuleAddressAssign {
		t.Fatalf("type=%d", typ)
	}
	ln, n2, err := quicvarint.Parse(wire[n1:])
	if err != nil {
		t.Fatal(err)
	}
	innerStart := n1 + n2
	innerEnd := innerStart + int(ln)
	if innerEnd > len(wire) {
		t.Fatal("short wire")
	}
	got, err := ParseAddressAssign(wire[innerStart:innerEnd])
	if err != nil || len(got) != 1 || !got[0].IP.Equal(addrs[0].IP) {
		t.Fatalf("err=%v got=%+v", err, got)
	}
}
