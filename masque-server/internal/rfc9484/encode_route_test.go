package rfc9484

import (
	"net"
	"testing"
)

func TestEncodeRouteAdvertisementPayloadRoundTrip(t *testing.T) {
	rg := IPRange{
		IPVersion: 4,
		Start:     net.ParseIP("10.0.0.0").To4(),
		End:       net.ParseIP("10.0.255.255").To4(),
		Protocol:  0,
	}
	inner, err := EncodeRouteAdvertisementPayload([]IPRange{rg})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseRouteAdvertisement(inner)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].IPVersion != 4 || got[0].Protocol != 0 {
		t.Fatalf("meta: %+v", got[0])
	}
	if !got[0].Start.Equal(rg.Start) || !got[0].End.Equal(rg.End) {
		t.Fatalf("range: %+v want start=%v end=%v", got[0], rg.Start, rg.End)
	}
	full, err := EncodeRouteAdvertisement([]IPRange{rg})
	if err != nil {
		t.Fatal(err)
	}
	if len(full) < len(inner)+2 {
		t.Fatalf("capsule wrapper too short: %d vs inner %d", len(full), len(inner))
	}
}

func TestIPv4CIDRToIPRange(t *testing.T) {
	rg, err := IPv4CIDRToIPRange("192.0.2.0/28")
	if err != nil {
		t.Fatal(err)
	}
	if rg.IPVersion != 4 || rg.Protocol != 0 {
		t.Fatalf("%+v", rg)
	}
	if !rg.Start.Equal(net.ParseIP("192.0.2.0")) || !rg.End.Equal(net.ParseIP("192.0.2.15")) {
		t.Fatalf("start=%v end=%v", rg.Start, rg.End)
	}
}
