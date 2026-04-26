package http3stub

import (
	"encoding/binary"
	"net"
)

// parseConnectIPDatagramDestination treats payload as a raw IPv4/IPv6 packet (after any RFC 9484 Context ID peel).
// If ok is false, the payload is not a minimal parseable IP packet (opaque bytes, probes, etc.).
func parseConnectIPDatagramDestination(b []byte) (dst net.IP, proto string, dstPort int, ok bool) {
	if len(b) < 1 {
		return nil, "", 0, false
	}
	switch b[0] >> 4 {
	case 4:
		return parseIPv4Datagram(b)
	case 6:
		return parseIPv6Datagram(b)
	default:
		return nil, "", 0, false
	}
}

func ipProtoString(p uint8) string {
	switch p {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 58:
		return "ipv6-icmp"
	default:
		return ""
	}
}

func parseIPv4Datagram(b []byte) (dst net.IP, proto string, dstPort int, ok bool) {
	if len(b) < 20 {
		return nil, "", 0, false
	}
	ihl := int(b[0]&0x0f) * 4
	if ihl < 20 || ihl > len(b) {
		return nil, "", 0, false
	}
	total := int(binary.BigEndian.Uint16(b[2:4]))
	if total < ihl || total > len(b) {
		return nil, "", 0, false
	}
	p := b[9]
	dst = net.IPv4(b[16], b[17], b[18], b[19]).To4()
	if dst == nil {
		return nil, "", 0, false
	}
	proto = ipProtoString(p)
	switch proto {
	case "tcp", "udp":
		if len(b) < ihl+4 {
			return dst, proto, 0, true
		}
		dstPort = int(binary.BigEndian.Uint16(b[ihl+2 : ihl+4]))
		return dst, proto, dstPort, true
	default:
		return dst, proto, 0, true
	}
}

// ipv6SkippableExtension reports whether nh is a header we walk past using RFC 8200 length rules.
// Fragment (44) uses a fixed 8-octet layout; 0, 43, 60 use (Hdr Ext Len + 1) * 8 octets.
func ipv6SkippableExtension(nh uint8) bool {
	switch nh {
	case 0, 43, 44, 60:
		return true
	default:
		return false
	}
}

// parseIPv6Datagram parses the IPv6 fixed header (destination in bytes 24–39), walks common extension
// headers (Hop-by-Hop, Routing, Destination, Fragment), then reads TCP/UDP destination port at the
// final payload offset. ESP/AH and deeper chains are out of scope for this stub.
func parseIPv6Datagram(b []byte) (dst net.IP, proto string, dstPort int, ok bool) {
	if len(b) < 40 {
		return nil, "", 0, false
	}
	dst = make(net.IP, 16)
	copy(dst, b[24:40])

	nh := b[6]
	off := 40
	for i := 0; i < 24 && ipv6SkippableExtension(nh); i++ {
		if len(b) < off+2 {
			return nil, "", 0, false
		}
		nextNH := b[off]
		if nh == 44 {
			if len(b) < off+8 {
				return nil, "", 0, false
			}
			off += 8
			nh = nextNH
			continue
		}
		extLen := int(b[off+1])
		hdrLen := (extLen + 1) * 8
		if hdrLen < 8 || off+hdrLen > len(b) {
			return nil, "", 0, false
		}
		off += hdrLen
		nh = nextNH
	}

	proto = ipProtoString(nh)
	switch proto {
	case "tcp", "udp":
		if len(b) < off+4 {
			return dst, proto, 0, true
		}
		dstPort = int(binary.BigEndian.Uint16(b[off+2 : off+4]))
		return dst, proto, dstPort, true
	default:
		return dst, proto, 0, true
	}
}
