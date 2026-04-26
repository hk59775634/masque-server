package http3stub

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go/http3"
)

const (
	udpRelayDialTimeout = 3 * time.Second
	udpRelayReadTimeout = 3 * time.Second
)

// maybeRelayConnectIPv4UDP performs an optional IPv4/UDP round-trip to the inner destination when
// CONNECT_IP_UDP_RELAY is enabled. It returns true if the datagram must not be echoed (reply sent,
// or relay failed and the original packet should be dropped). Returns false to echo the original
// RFC 9297 payload unchanged (non-IPv4-UDP, fragmented, or relay disabled).
func maybeRelayConnectIPv4UDP(ctx context.Context, str *http3.Stream, cfg ListenConfig, innerIP []byte) bool {
	if !cfg.ConnectIPUDPRelay {
		return false
	}
	reply, status := relayIPv4UDPDatagram(ctx, innerIP)
	switch status {
	case udpRelayNotApplicable:
		return false
	case udpRelayOK:
		out := encodeRFC9484ContextZeroIPPacket(reply)
		if err := str.SendDatagram(out); err != nil {
			if cfg.ConnectIPUDPRelayErrors != nil {
				cfg.ConnectIPUDPRelayErrors.WithLabelValues("send").Inc()
			}
			return true
		}
		if cfg.ConnectIPUDPRelayReplies != nil {
			cfg.ConnectIPUDPRelayReplies.Inc()
		}
		if cfg.ConnectIPDatagramsSent != nil {
			cfg.ConnectIPDatagramsSent.Inc()
		}
		return true
	default:
		if cfg.ConnectIPUDPRelayErrors != nil {
			cfg.ConnectIPUDPRelayErrors.WithLabelValues(string(status)).Inc()
		}
		return true
	}
}

type udpRelayStatus string

const (
	udpRelayNotApplicable udpRelayStatus = "na"
	udpRelayOK            udpRelayStatus = "ok"
	udpRelayParse         udpRelayStatus = "parse"
	udpRelayDial          udpRelayStatus = "dial"
	udpRelayRead          udpRelayStatus = "read"
	udpRelayBuild         udpRelayStatus = "build"
)

// relayIPv4UDPDatagram dials the inner IPv4/UDP destination, sends the UDP payload, and builds a
// reply IPv4/UDP packet (src/dst and ports swapped). status na means caller should fall back to echo.
func relayIPv4UDPDatagram(ctx context.Context, ip []byte) (reply []byte, status udpRelayStatus) {
	srcIP, dstIP, sport, dport, payload, ok := parseIPv4UDPDatagram(ip)
	if !ok {
		return nil, udpRelayNotApplicable
	}
	if len(payload) > maxConnectIPDatagramBytes-28 {
		return nil, udpRelayParse
	}

	dctx, cancel := context.WithTimeout(ctx, udpRelayDialTimeout)
	defer cancel()

	addr := &net.UDPAddr{IP: dstIP, Port: int(dport)}
	d := net.Dialer{}
	conn, err := d.DialContext(dctx, "udp", addr.String())
	if err != nil {
		return nil, udpRelayDial
	}
	defer conn.Close()

	if _, err := conn.Write(payload); err != nil {
		return nil, udpRelayDial
	}

	_ = conn.SetReadDeadline(time.Now().Add(udpRelayReadTimeout))
	buf := make([]byte, maxConnectIPDatagramBytes)
	n, err := conn.Read(buf)
	if err != nil || n <= 0 {
		return nil, udpRelayRead
	}

	reply, err = buildIPv4UDPReply(srcIP, dstIP, sport, dport, buf[:n])
	if err != nil {
		return nil, udpRelayBuild
	}
	return reply, udpRelayOK
}

// parseIPv4UDPDatagram extracts relay fields from a single unfragmented IPv4/UDP packet.
func parseIPv4UDPDatagram(ip []byte) (srcIP, dstIP net.IP, sport, dport uint16, payload []byte, ok bool) {
	if len(ip) < 28 {
		return nil, nil, 0, 0, nil, false
	}
	if ip[0]>>4 != 4 {
		return nil, nil, 0, 0, nil, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || ihl > len(ip) {
		return nil, nil, 0, 0, nil, false
	}
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	if total < ihl+8 || total > len(ip) {
		return nil, nil, 0, 0, nil, false
	}
	if ip[9] != 17 {
		return nil, nil, 0, 0, nil, false
	}
	// Drop IP fragments (offset non-zero or more-fragments set).
	frag := binary.BigEndian.Uint16(ip[6:8])
	if frag&0x1fff != 0 || frag&0x2000 != 0 {
		return nil, nil, 0, 0, nil, false
	}
	srcIP = net.IPv4(ip[12], ip[13], ip[14], ip[15]).To4()
	dstIP = net.IPv4(ip[16], ip[17], ip[18], ip[19]).To4()
	if srcIP == nil || dstIP == nil {
		return nil, nil, 0, 0, nil, false
	}
	sport = binary.BigEndian.Uint16(ip[ihl : ihl+2])
	dport = binary.BigEndian.Uint16(ip[ihl+2 : ihl+4])
	udpLen := int(binary.BigEndian.Uint16(ip[ihl+4 : ihl+6]))
	if udpLen < 8 || ihl+udpLen > total {
		return nil, nil, 0, 0, nil, false
	}
	payload = make([]byte, udpLen-8)
	copy(payload, ip[ihl+8:ihl+udpLen])
	return srcIP, dstIP, sport, dport, payload, true
}

func buildIPv4UDPReply(origSrc, origDst net.IP, origSport, origDport uint16, replyPayload []byte) ([]byte, error) {
	udpLen := 8 + len(replyPayload)
	total := 20 + udpLen
	if total > maxConnectIPDatagramBytes {
		return nil, fmt.Errorf("reply too large")
	}
	out := make([]byte, total)
	out[0] = 0x45
	out[1] = 0
	binary.BigEndian.PutUint16(out[2:4], uint16(total))
	var id [2]byte
	if _, err := rand.Read(id[:]); err != nil {
		binary.BigEndian.PutUint16(out[4:6], uint16(time.Now().UnixNano()))
	} else {
		copy(out[4:6], id[:])
	}
	binary.BigEndian.PutUint16(out[6:8], 0)
	out[8] = 64
	out[9] = 17
	copy(out[12:16], origDst.To4())
	copy(out[16:20], origSrc.To4())
	ihl := 20
	binary.BigEndian.PutUint16(out[ihl:ihl+2], origDport)
	binary.BigEndian.PutUint16(out[ihl+2:ihl+4], origSport)
	binary.BigEndian.PutUint16(out[ihl+4:ihl+6], uint16(udpLen))
	binary.BigEndian.PutUint16(out[ihl+6:ihl+8], 0)
	copy(out[ihl+8:], replyPayload)

	ck := ipv4HeaderChecksum(out[:ihl])
	binary.BigEndian.PutUint16(out[10:12], ck)

	uSum := udpIPv4Checksum(origDst, origSrc, out[ihl:ihl+udpLen])
	binary.BigEndian.PutUint16(out[ihl+6:ihl+8], uSum)
	return out, nil
}

func ipv4HeaderChecksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i < len(hdr); i += 2 {
		if i == 10 {
			continue
		}
		sum += uint32(binary.BigEndian.Uint16(hdr[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func udpIPv4Checksum(src, dst net.IP, udp []byte) uint16 {
	s, d := src.To4(), dst.To4()
	if s == nil || d == nil {
		return 0
	}
	var sum uint32
	sum += uint32(binary.BigEndian.Uint16(s[0:2]))
	sum += uint32(binary.BigEndian.Uint16(s[2:4]))
	sum += uint32(binary.BigEndian.Uint16(d[0:2]))
	sum += uint32(binary.BigEndian.Uint16(d[2:4]))
	sum += uint32(17)
	sum += uint32(len(udp))
	for i := 0; i+1 < len(udp); i += 2 {
		if i == 6 {
			continue
		}
		sum += uint32(binary.BigEndian.Uint16(udp[i : i+2]))
	}
	if len(udp)%2 == 1 {
		sum += uint32(udp[len(udp)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	cs := ^uint16(sum)
	if cs == 0 {
		return 0xffff
	}
	return cs
}
