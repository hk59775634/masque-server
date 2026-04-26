package http3stub

import (
	"encoding/binary"
	"net"
	"testing"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func TestParseIPv4ICMPEchoRequest(t *testing.T) {
	ihl := 20
	icmpBody, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: 0x1122, Seq: 0x3344, Data: []byte("ping")},
	}).Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	total := ihl + len(icmpBody)
	ip := make([]byte, total)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(total))
	ip[8] = 64
	ip[9] = 1
	copy(ip[12:16], []byte{10, 1, 2, 3})
	copy(ip[16:20], []byte{8, 8, 8, 8})
	copy(ip[ihl:], icmpBody)
	ck := ipv4HeaderChecksum(ip[:ihl])
	binary.BigEndian.PutUint16(ip[10:12], ck)

	src, dst, id, seq, data, ok := parseIPv4ICMPEchoRequest(ip)
	if !ok {
		t.Fatal("parse failed")
	}
	if src.String() != "10.1.2.3" || dst.String() != "8.8.8.8" {
		t.Fatalf("addr src=%s dst=%s", src, dst)
	}
	if id != 0x1122 || seq != 0x3344 {
		t.Fatalf("id=%#x seq=%#x", id, seq)
	}
	if string(data) != "ping" {
		t.Fatalf("data %q", data)
	}
}

func TestBuildIPv4ICMPPacketRoundTrip(t *testing.T) {
	inner, err := (&icmp.Message{
		Type: ipv4.ICMPTypeEchoReply,
		Code: 0,
		Body: &icmp.Echo{ID: 1, Seq: 2, Data: []byte{0xab}},
	}).Marshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	src := net.IPv4(8, 8, 8, 8)
	dst := net.IPv4(10, 0, 0, 1)
	pkt, err := buildIPv4ICMPPacket(src, dst, inner)
	if err != nil {
		t.Fatal(err)
	}
	if !ipv4HeaderChecksumValid(pkt[:20]) {
		t.Fatal("bad IPv4 checksum")
	}
	if pkt[9] != 1 {
		t.Fatalf("proto %d", pkt[9])
	}
	if !net.IPv4(pkt[12], pkt[13], pkt[14], pkt[15]).Equal(src) {
		t.Fatal("src ip")
	}
	if !net.IPv4(pkt[16], pkt[17], pkt[18], pkt[19]).Equal(dst) {
		t.Fatal("dst ip")
	}
	rm, err := icmp.ParseMessage(1, pkt[20:])
	if err != nil {
		t.Fatal(err)
	}
	if rm.Type != ipv4.ICMPTypeEchoReply {
		t.Fatalf("type %v", rm.Type)
	}
}
