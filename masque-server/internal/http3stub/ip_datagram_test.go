package http3stub

import (
	"encoding/binary"
	"net"
	"testing"

	"afbuyers/masque-server/internal/auth"
)

func TestParseIPv4UDP(t *testing.T) {
	// IPv4 header (20) + UDP (8), dst 198.51.100.7, dport 443
	var b []byte
	b = append(b, 0x45, 0x00)
	total := uint16(20 + 8)
	b = binary.BigEndian.AppendUint16(b, total)
	b = append(b, 0x00, 0x00, 0x00, 0x00)
	b = append(b, 64, 17)          // TTL, UDP
	b = append(b, 0x00, 0x00)      // checksum placeholder
	b = append(b, 10, 0, 0, 1)     // src
	b = append(b, 198, 51, 100, 7) // dst
	// UDP
	b = append(b, 0x12, 0x34, 0x01, 0xbb) // sport, dport=443
	b = binary.BigEndian.AppendUint16(b, 8)
	b = append(b, 0x00, 0x00)

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("expected ok")
	}
	if proto != "udp" || port != 443 {
		t.Fatalf("proto=%q port=%d", proto, port)
	}
	if !dst.Equal(net.ParseIP("198.51.100.7")) {
		t.Fatalf("dst=%v", dst)
	}
}

func TestParseNonIP(t *testing.T) {
	_, _, _, ok := parseConnectIPDatagramDestination([]byte("msq-dgram"))
	if ok {
		t.Fatal("expected non-IP")
	}
}

func TestParseIPv4ACLAllowed(t *testing.T) {
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "198.51.100.0/24", "protocol": "udp", "port": "any"},
		},
	}
	var b []byte
	b = append(b, 0x45, 0x00)
	b = binary.BigEndian.AppendUint16(b, 28)
	b = append(b, 0x00, 0x00, 0x00, 0x00)
	b = append(b, 64, 17)
	b = append(b, 0x00, 0x00)
	b = append(b, 10, 0, 0, 1)
	b = append(b, 198, 51, 100, 7)
	b = append(b, 0x12, 0x34, 0x00, 0x35)
	b = binary.BigEndian.AppendUint16(b, 8)
	b = append(b, 0x00, 0x00)

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("parse")
	}
	if !auth.IsAllowed(acl, dst.String(), proto, port) {
		t.Fatalf("expected allowed dst=%s proto=%s port=%d", dst, proto, port)
	}
}

func TestParseIPv6UDPDirect(t *testing.T) {
	// 40-byte IPv6 + 8-byte UDP, Next Header = UDP (17) in main header
	b := make([]byte, 48)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:6], 8)
	b[6] = 17
	b[7] = 64
	copy(b[8:24], net.ParseIP("fc00::1").To16())
	copy(b[24:40], net.ParseIP("2001:db8::77").To16())
	binary.BigEndian.PutUint16(b[40:42], 0x1234)
	binary.BigEndian.PutUint16(b[42:44], 53)
	binary.BigEndian.PutUint16(b[44:46], 8)
	b[46], b[47] = 0, 0

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("parse")
	}
	if proto != "udp" || port != 53 {
		t.Fatalf("proto=%q port=%d", proto, port)
	}
	if !dst.Equal(net.ParseIP("2001:db8::77")) {
		t.Fatalf("dst=%v", dst)
	}
}

func TestParseIPv6UDPOverHopByHop(t *testing.T) {
	// IPv6 main Next Header = 0 (HBH), 8-byte HBH with Next=UDP, then 8-byte UDP
	b := make([]byte, 56)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:6], 16) // 8 + 8
	b[6] = 0                               // Hop-by-Hop
	b[7] = 64
	copy(b[8:24], net.ParseIP("fc00::1").To16())
	copy(b[24:40], net.ParseIP("2001:db8::2").To16())
	// HBH at 40
	b[40] = 17 // UDP follows
	b[41] = 0  // 8 octets total
	// UDP at 48
	binary.BigEndian.PutUint16(b[48:50], 0x1234)
	binary.BigEndian.PutUint16(b[50:52], 443)
	binary.BigEndian.PutUint16(b[52:54], 8)
	b[54], b[55] = 0, 0

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("parse")
	}
	if proto != "udp" || port != 443 {
		t.Fatalf("proto=%q port=%d", proto, port)
	}
	if !dst.Equal(net.ParseIP("2001:db8::2")) {
		t.Fatalf("dst=%v", dst)
	}
}

func TestParseIPv6UDPOverHBHAndDestination(t *testing.T) {
	// Main nh=0 (HBH), HBH next=60 (Destination), Dest next=17 (UDP)
	b := make([]byte, 64)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:6], 24) // 8+8+8
	b[6] = 0
	b[7] = 64
	copy(b[8:24], net.ParseIP("fc00::1").To16())
	copy(b[24:40], net.ParseIP("2001:db8::3").To16())
	// HBH
	b[40] = 60
	b[41] = 0
	// Destination options at 48
	b[48] = 17
	b[49] = 0
	// UDP at 56
	binary.BigEndian.PutUint16(b[56:58], 0x1111)
	binary.BigEndian.PutUint16(b[58:60], 5353)
	binary.BigEndian.PutUint16(b[60:62], 8)
	b[62], b[63] = 0, 0

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("parse")
	}
	if proto != "udp" || port != 5353 {
		t.Fatalf("proto=%q port=%d", proto, port)
	}
	if !dst.Equal(net.ParseIP("2001:db8::3")) {
		t.Fatalf("dst=%v", dst)
	}
}

func TestParseIPv4ACLDenied(t *testing.T) {
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "198.51.100.0/24", "protocol": "udp", "port": "any"},
		},
	}
	var b []byte
	b = append(b, 0x45, 0x00)
	b = binary.BigEndian.AppendUint16(b, 28)
	b = append(b, 0x00, 0x00, 0x00, 0x00)
	b = append(b, 64, 17)
	b = append(b, 0x00, 0x00)
	b = append(b, 10, 0, 0, 1)
	b = append(b, 8, 8, 8, 8)
	b = append(b, 0x12, 0x34, 0x00, 0x35)
	b = binary.BigEndian.AppendUint16(b, 8)
	b = append(b, 0x00, 0x00)

	dst, proto, port, ok := parseConnectIPDatagramDestination(b)
	if !ok {
		t.Fatal("parse")
	}
	if auth.IsAllowed(acl, dst.String(), proto, port) {
		t.Fatal("expected denied")
	}
}
