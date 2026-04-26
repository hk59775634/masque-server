package http3stub

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const icmpRelayTimeout = 3 * time.Second

// maybeRelayConnectIPv4ICMP relays a single IPv4 ICMP Echo Request when CONNECT_IP_ICMP_RELAY is enabled.
// Returns true if the datagram must not be echoed (reply sent or relay failed and original dropped).
func maybeRelayConnectIPv4ICMP(ctx context.Context, str *http3.Stream, cfg ListenConfig, innerIP []byte) bool {
	if !cfg.ConnectIPICMPRelay {
		return false
	}
	reply, status := relayIPv4ICMPEcho(ctx, innerIP)
	switch status {
	case icmpRelayNotApplicable:
		return false
	case icmpRelayOK:
		out := encodeRFC9484ContextZeroIPPacket(reply)
		if err := str.SendDatagram(out); err != nil {
			if cfg.ConnectIPICMPRelayErrors != nil {
				cfg.ConnectIPICMPRelayErrors.WithLabelValues("send").Inc()
			}
			return true
		}
		if cfg.ConnectIPICMPRelayReplies != nil {
			cfg.ConnectIPICMPRelayReplies.Inc()
		}
		if cfg.ConnectIPDatagramsSent != nil {
			cfg.ConnectIPDatagramsSent.Inc()
		}
		return true
	default:
		if cfg.ConnectIPICMPRelayErrors != nil {
			cfg.ConnectIPICMPRelayErrors.WithLabelValues(string(status)).Inc()
		}
		return true
	}
}

type icmpRelayStatus string

const (
	icmpRelayNotApplicable icmpRelayStatus = "na"
	icmpRelayOK            icmpRelayStatus = "ok"
	icmpRelayParse         icmpRelayStatus = "parse"
	icmpRelayDial          icmpRelayStatus = "dial"
	icmpRelayRead          icmpRelayStatus = "read"
	icmpRelayBuild         icmpRelayStatus = "build"
	icmpRelayMismatch      icmpRelayStatus = "mismatch"
)

func relayIPv4ICMPEcho(ctx context.Context, ip []byte) (reply []byte, status icmpRelayStatus) {
	srcIP, dstIP, id, seq, echoData, ok := parseIPv4ICMPEchoRequest(ip)
	if !ok {
		return nil, icmpRelayNotApplicable
	}
	if len(echoData) > maxConnectIPDatagramBytes-28 {
		return nil, icmpRelayParse
	}

	dctx, cancel := context.WithTimeout(ctx, icmpRelayTimeout)
	defer cancel()

	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, icmpRelayDial
	}
	defer c.Close()

	deadline, ok := dctx.Deadline()
	if !ok {
		deadline = time.Now().Add(icmpRelayTimeout)
	}
	if err := c.SetDeadline(deadline); err != nil {
		return nil, icmpRelayDial
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   int(id),
			Seq:  int(seq),
			Data: echoData,
		},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return nil, icmpRelayParse
	}
	if _, err := c.WriteTo(wb, &net.IPAddr{IP: dstIP}); err != nil {
		return nil, icmpRelayDial
	}

	rb := make([]byte, maxConnectIPDatagramBytes)
	n, _, err := c.ReadFrom(rb)
	if err != nil || n <= 0 {
		return nil, icmpRelayRead
	}

	rm, err := icmp.ParseMessage(1, rb[:n])
	if err != nil {
		return nil, icmpRelayRead
	}
	if rm.Type != ipv4.ICMPTypeEchoReply || rm.Code != 0 {
		return nil, icmpRelayMismatch
	}
	echo, ok := rm.Body.(*icmp.Echo)
	if !ok || echo.ID != int(id) || echo.Seq != int(seq) {
		return nil, icmpRelayMismatch
	}
	inner, err := rm.Marshal(nil)
	if err != nil {
		return nil, icmpRelayRead
	}
	out, err := buildIPv4ICMPPacket(dstIP, srcIP, inner)
	if err != nil {
		return nil, icmpRelayBuild
	}
	return out, icmpRelayOK
}

// parseIPv4ICMPEchoRequest extracts Echo Request (type 8) from an unfragmented IPv4 packet.
func parseIPv4ICMPEchoRequest(ip []byte) (srcIP, dstIP net.IP, id, seq uint16, echoData []byte, ok bool) {
	if len(ip) < 20+8 {
		return nil, nil, 0, 0, nil, false
	}
	if ip[0]>>4 != 4 {
		return nil, nil, 0, 0, nil, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || ihl+8 > len(ip) {
		return nil, nil, 0, 0, nil, false
	}
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	if total < ihl+8 || total > len(ip) {
		return nil, nil, 0, 0, nil, false
	}
	if ip[9] != 1 { // ICMP
		return nil, nil, 0, 0, nil, false
	}
	ff := binary.BigEndian.Uint16(ip[6:8])
	if ff&0x1fff != 0 || ff&0x2000 != 0 {
		return nil, nil, 0, 0, nil, false
	}
	srcIP = net.IPv4(ip[12], ip[13], ip[14], ip[15]).To4()
	dstIP = net.IPv4(ip[16], ip[17], ip[18], ip[19]).To4()
	if srcIP == nil || dstIP == nil {
		return nil, nil, 0, 0, nil, false
	}
	icmpOff := ihl
	if ip[icmpOff] != 8 || ip[icmpOff+1] != 0 {
		return nil, nil, 0, 0, nil, false
	}
	id = binary.BigEndian.Uint16(ip[icmpOff+4 : icmpOff+6])
	seq = binary.BigEndian.Uint16(ip[icmpOff+6 : icmpOff+8])
	if total < icmpOff+8 {
		return nil, nil, 0, 0, nil, false
	}
	echoData = make([]byte, total-icmpOff-8)
	copy(echoData, ip[icmpOff+8:total])
	return srcIP, dstIP, id, seq, echoData, true
}

func buildIPv4ICMPPacket(srcIP, dstIP net.IP, icmpPayload []byte) ([]byte, error) {
	total := 20 + len(icmpPayload)
	if total > maxConnectIPDatagramBytes {
		return nil, fmt.Errorf("icmp reply too large")
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
	out[9] = 1
	copy(out[12:16], srcIP.To4())
	copy(out[16:20], dstIP.To4())
	copy(out[20:], icmpPayload)
	ck := ipv4HeaderChecksum(out[:20])
	binary.BigEndian.PutUint16(out[10:12], ck)
	return out, nil
}
