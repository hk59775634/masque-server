package http3stub

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestParseIPv4UDPDatagram(t *testing.T) {
	// Minimal IPv4/UDP: 10.0.0.2:40000 -> 192.0.2.1:53, payload "ab"
	ip := make([]byte, 20+8+2)
	ip[0] = 0x45
	ip[8] = 64
	ip[9] = 17
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)))
	copy(ip[12:16], []byte{10, 0, 0, 2})
	copy(ip[16:20], []byte{192, 0, 2, 1})
	ihl := 20
	binary.BigEndian.PutUint16(ip[ihl:ihl+2], 40000)
	binary.BigEndian.PutUint16(ip[ihl+2:ihl+4], 53)
	binary.BigEndian.PutUint16(ip[ihl+4:ihl+6], 8+2)
	copy(ip[ihl+8:], "ab")

	src, dst, sp, dp, pl, ok := parseIPv4UDPDatagram(ip)
	if !ok {
		t.Fatal("parse failed")
	}
	if src.String() != "10.0.0.2" || dst.String() != "192.0.2.1" {
		t.Fatalf("addr src=%s dst=%s", src, dst)
	}
	if sp != 40000 || dp != 53 {
		t.Fatalf("ports sport=%d dport=%d", sp, dp)
	}
	if string(pl) != "ab" {
		t.Fatalf("payload %q", pl)
	}
}

func TestBuildIPv4UDPReplyHeadersAndChecksums(t *testing.T) {
	origSrc := net.IPv4(10, 0, 0, 2)
	origDst := net.IPv4(192, 0, 2, 1)
	reply, err := buildIPv4UDPReply(origSrc, origDst, 40000, 53, []byte("xy"))
	if err != nil {
		t.Fatal(err)
	}
	if !ipv4HeaderChecksumValid(reply[:20]) {
		t.Fatal("invalid IPv4 header checksum")
	}
	if !udpChecksumValid(origDst, origSrc, reply[20:]) {
		t.Fatal("invalid UDP checksum")
	}
	src, dst, sp, dp, pl, ok := parseIPv4UDPDatagram(reply)
	if !ok {
		t.Fatal("parse built packet")
	}
	if src.String() != "192.0.2.1" || dst.String() != "10.0.0.2" {
		t.Fatalf("swap src=%s dst=%s", src, dst)
	}
	if sp != 53 || dp != 40000 {
		t.Fatalf("swap ports sport=%d dport=%d", sp, dp)
	}
	if string(pl) != "xy" {
		t.Fatalf("payload %q", pl)
	}
}

func TestParseIPv4UDPDatagramRejectsFragment(t *testing.T) {
	ip := make([]byte, 40)
	ip[0] = 0x45
	ip[9] = 17
	binary.BigEndian.PutUint16(ip[2:4], 40)
	// MF set
	binary.BigEndian.PutUint16(ip[6:8], 0x2000)
	if _, _, _, _, _, ok := parseIPv4UDPDatagram(ip); ok {
		t.Fatal("expected fragment reject")
	}
}

func ipv4HeaderChecksumValid(hdr []byte) bool {
	if len(hdr) < 20 {
		return false
	}
	var sum uint32
	for i := 0; i < 20; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum) == 0
}

func udpChecksumValid(src4, dst4 net.IP, udp []byte) bool {
	// Sum pseudo-header + UDP (including checksum field) should fold to 0xffff.
	s, d := src4.To4(), dst4.To4()
	if s == nil || d == nil || len(udp) < 8 {
		return false
	}
	var sum uint32
	sum += uint32(binary.BigEndian.Uint16(s[0:2]))
	sum += uint32(binary.BigEndian.Uint16(s[2:4]))
	sum += uint32(binary.BigEndian.Uint16(d[0:2]))
	sum += uint32(binary.BigEndian.Uint16(d[2:4]))
	sum += uint32(17)
	sum += uint32(len(udp))
	for i := 0; i+1 < len(udp); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(udp[i : i+2]))
	}
	if len(udp)%2 == 1 {
		sum += uint32(udp[len(udp)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum) == 0
}
