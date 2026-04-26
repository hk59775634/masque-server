package rfc9484

import (
	"testing"

	"github.com/quic-go/quic-go/quicvarint"
)

func TestParseRouteAdvertisementRFCExample(t *testing.T) {
	// IP Version=4, 0.0.0.0, 255.255.255.255, protocol 0
	var p []byte
	p = append(p, 4)
	p = append(p, make([]byte, 4)...)
	p = append(p, 255, 255, 255, 255)
	p = append(p, 0)

	ranges, err := ParseRouteAdvertisement(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 1 || ranges[0].Protocol != 0 {
		t.Fatalf("%+v", ranges)
	}
}

func TestParseAddressRequestRejectZeroRequests(t *testing.T) {
	_, err := ParseAddressRequest(nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseAddressRequestOne(t *testing.T) {
	var p []byte
	p = quicvarint.Append(p, 1) // request id
	p = append(p, 4)
	p = append(p, make([]byte, 4)...) // 0.0.0.0
	p = append(p, 32)

	got, err := ParseAddressRequest(p)
	if err != nil || len(got) != 1 || got[0].RequestID != 1 {
		t.Fatalf("err=%v got=%+v", err, got)
	}
}
