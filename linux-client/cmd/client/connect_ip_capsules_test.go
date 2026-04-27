package main

import (
	"net"
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestEncodeAddressRequestCapsuleRoundTrip(t *testing.T) {
	wire := encodeAddressRequestIPv4Unspecified(99)
	typ, payload, consumed, err := parseOneCapsule9297(wire, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if typ != capsuleAddressRequest {
		t.Fatalf("type %d", typ)
	}
	if consumed != len(wire) {
		t.Fatalf("consumed %d len %d", consumed, len(wire))
	}
	reqID, n, err := quicvarint.Parse(payload)
	if err != nil || reqID != 99 {
		t.Fatalf("reqID=%d err=%v", reqID, err)
	}
	if n >= len(payload) || payload[n] != 4 {
		t.Fatalf("payload after varint: %#x", payload[n:])
	}
}

func TestRouteRangeToCIDR(t *testing.T) {
	cidr, ok := routeRangeToCIDR(net.IPv4(10, 0, 0, 0), net.IPv4(10, 0, 0, 255))
	if !ok || cidr != "10.0.0.0/24" {
		t.Fatalf("cidr=%q ok=%v", cidr, ok)
	}
	if _, ok := routeRangeToCIDR(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 254)); ok {
		t.Fatal("expected non-cidr range to fail")
	}
}
