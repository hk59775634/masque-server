package main

import (
	"encoding/binary"
	"testing"
)

func TestQuicUDPAddrFromCapabilities(t *testing.T) {
	raw := []byte(`{
  "transport": {
    "http3_stub": {"listen_udp": ":8444"}
  }
}`)
	out, err := quicUDPAddrFromCapabilities(raw, "http://127.0.0.1:8443")
	if err != nil {
		t.Fatal(err)
	}
	if out != "127.0.0.1:8444" {
		t.Fatalf("got %q want 127.0.0.1:8444", out)
	}
}

func TestDoctorRFC9484IPv4UDPProbePacket(t *testing.T) {
	p := doctorRFC9484IPv4UDPProbePacket()
	if len(p) != 1+28 {
		t.Fatalf("len=%d want 29", len(p))
	}
	if p[0] != 0x00 {
		t.Fatalf("context id varint want single 0 byte, got %#x", p[0])
	}
	if p[1]>>4 != 4 {
		t.Fatalf("want IPv4, got %#x", p[1])
	}
	if int(binary.BigEndian.Uint16(p[3:5])) != 28 {
		t.Fatalf("IPv4 total length")
	}
	if p[20] != 1 || p[19] != 2 || p[18] != 0 || p[17] != 192 {
		t.Fatalf("dst IPv4 wrong: %v", p[17:21])
	}
}

func TestRFC9484Context0RoundTrip(t *testing.T) {
	ip := []byte{0x45, 0x00, 0x00, 0x14, 0x00, 0x00, 0x00, 0x00, 0x40, 0xff, 0x00, 0x00, 10, 0, 0, 1, 10, 0, 0, 2}
	fr := rfc9484PrependContext0(ip)
	got, err := rfc9484StripContext0(fr)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(ip) {
		t.Fatalf("round-trip mismatch")
	}
}

func TestQuicUDPAddrFromCapabilitiesFullHost(t *testing.T) {
	raw := []byte(`{"transport":{"http3_stub":{"listen_udp":"192.0.2.1:9444"}}}`)
	out, err := quicUDPAddrFromCapabilities(raw, "http://ignored")
	if err != nil {
		t.Fatal(err)
	}
	if out != "192.0.2.1:9444" {
		t.Fatalf("got %q", out)
	}
}
