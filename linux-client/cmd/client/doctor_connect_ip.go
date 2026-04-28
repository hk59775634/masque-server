package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/quic-go/quic-go/quicvarint"
)

const masqueConnectIPProto = "connect-ip"

// quicUDPAddrFromCapabilities resolves the QUIC UDP dial target from GET /v1/masque/capabilities.
// It accepts either legacy transport.http3_stub.listen_udp or tunnel.quic.listen_udp_addr (e.g. ":8444").
func quicUDPAddrFromCapabilities(capJSON []byte, masqueBaseURL string) (string, error) {
	var doc map[string]any
	if err := json.Unmarshal(capJSON, &doc); err != nil {
		return "", err
	}
	var listenUDP string
	if tr, _ := doc["transport"].(map[string]any); tr != nil {
		if stub, _ := tr["http3_stub"].(map[string]any); stub != nil {
			listenUDP, _ = stub["listen_udp"].(string)
		}
	}
	if strings.TrimSpace(listenUDP) == "" {
		if tun, _ := doc["tunnel"].(map[string]any); tun != nil {
			if quic, _ := tun["quic"].(map[string]any); quic != nil {
				listenUDP, _ = quic["listen_udp_addr"].(string)
			}
		}
	}
	listenUDP = strings.TrimSpace(listenUDP)
	if listenUDP == "" {
		return "", fmt.Errorf("capabilities: QUIC UDP not advertised (set QUIC_LISTEN_ADDR on masque-server; expect transport.http3_stub.listen_udp or tunnel.quic.listen_udp_addr)")
	}

	host, port, err := net.SplitHostPort(listenUDP)
	if err != nil {
		return "", fmt.Errorf("parse listen_udp %q: %w", listenUDP, err)
	}
	if host != "" {
		return listenUDP, nil
	}

	base, err := url.Parse(masqueBaseURL)
	if err != nil {
		return "", err
	}
	h := base.Hostname()
	if h == "" {
		return "", fmt.Errorf("cannot derive host for listen_udp %q (masque URL has no host)", listenUDP)
	}
	return net.JoinHostPort(h, port), nil
}

// doctorRFC9484IPv4UDPProbePacket returns RFC 9484 Context ID 0 + minimal IPv4/UDP toward 192.0.2.1:53 (TEST-NET-1).
// Requires device ACL to allow that destination when CONNECT-IP auth is enabled.
func doctorRFC9484IPv4UDPProbePacket() []byte {
	var ip []byte
	ip = append(ip, 0x45, 0x00)
	ip = binary.BigEndian.AppendUint16(ip, 28)
	ip = append(ip, 0x00, 0x00, 0x00, 0x00)
	ip = append(ip, 64, 17, 0x00, 0x00)
	ip = append(ip, 192, 0, 2, 10)
	ip = append(ip, 192, 0, 2, 1)
	ip = append(ip, 0xde, 0xed, 0x00, 0x35)
	ip = binary.BigEndian.AppendUint16(ip, 8)
	ip = append(ip, 0x00, 0x00)
	out := quicvarint.Append(nil, 0)
	return append(out, ip...)
}

// doctorProbeConnectIP dials QUIC to udpHostPort and performs extended CONNECT :protocol connect-ip with optional auth headers.
// It uses OpenRequestStream so it can send/receive RFC 9297 HTTP Datagrams (stub echo smoke).
// When rfc9484UDP is true, sends a second datagram (RFC 9484 CID 0 + IPv4/UDP) and expects the same bytes echoed back.
func doctorProbeConnectIP(ctx context.Context, udpHostPort, deviceToken, fingerprint string, rfc9484UDP bool) error {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sess, err := dialConnectIP(dctx, udpHostPort, deviceToken, fingerprint, nil)
	if err != nil {
		return err
	}
	defer sess.Close()
	defer sess.Resp.Body.Close()

	rs := sess.RS

	// RFC 9484 Section 6: Context ID 0 (varint) + opaque probe (not an IP packet).
	payload := append([]byte{0x00}, []byte{0x6d, 0x73, 0x71, 0x2d, 0x64, 0x67, 0x72, 0x61, 0x6d}...) // "msq-dgram"
	if err := rs.SendDatagram(payload); err != nil {
		return fmt.Errorf("SendDatagram: %w", err)
	}
	recvCtx, recvCancel := context.WithTimeout(dctx, 3*time.Second)
	defer recvCancel()
	back, err := rs.ReceiveDatagram(recvCtx)
	if err != nil {
		return fmt.Errorf("ReceiveDatagram (echo): %w", err)
	}
	if !bytes.Equal(back, payload) {
		return fmt.Errorf("datagram echo mismatch: sent %d bytes, got %d bytes", len(payload), len(back))
	}

	if !rfc9484UDP {
		return nil
	}

	pkt := doctorRFC9484IPv4UDPProbePacket()
	if err := rs.SendDatagram(pkt); err != nil {
		return fmt.Errorf("SendDatagram (rfc9484 ipv4/udp): %w", err)
	}
	recvCtx2, recvCancel2 := context.WithTimeout(dctx, 3*time.Second)
	defer recvCancel2()
	back2, err := rs.ReceiveDatagram(recvCtx2)
	if err != nil {
		return fmt.Errorf("ReceiveDatagram (rfc9484 ipv4/udp echo): %w", err)
	}
	if !bytes.Equal(back2, pkt) {
		return fmt.Errorf("rfc9484 ipv4/udp echo mismatch: sent %d bytes, got %d bytes", len(pkt), len(back2))
	}

	return nil
}

func stripIPv6Brackets(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		if i := strings.Index(host, "]"); i > 1 {
			return host[1:i]
		}
	}
	return host
}
