package http3stub

import (
	"context"
	"log"

	"afbuyers/masque-server/internal/auth"
	"github.com/quic-go/quic-go/http3"
)

// maxConnectIPDatagramBytes bounds echo and relay payload size (stub; not a production router).
const maxConnectIPDatagramBytes = 1200

// runConnectIPDatagramEchoLoop receives HTTP Datagrams (RFC 9297) on the CONNECT-IP stream.
// RFC 9484 Section 6: payloads may start with a Context ID (QUIC varint); ID 0 wraps a full IP packet.
// IP-shaped inner payloads use the same device ACL as POST /connect; optional CONNECT_IP_UDP_RELAY may
// forward unfragmented IPv4/UDP and return the reply instead of echoing; optional CONNECT_IP_ICMP_RELAY does the
// same for IPv4 ICMP Echo; opaque / legacy payloads are echoed.
func runConnectIPDatagramEchoLoop(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any) {
	for {
		data, err := str.ReceiveDatagram(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("connect-ip stub: datagram recv end remote err=%v", err)
			return
		}
		if len(data) == 0 || len(data) > maxConnectIPDatagramBytes {
			if cfg.ConnectIPDatagramDrops != nil {
				cfg.ConnectIPDatagramDrops.Inc()
			}
			continue
		}
		if cfg.ConnectIPDatagramsReceived != nil {
			cfg.ConnectIPDatagramsReceived.Inc()
		}

		payload, drop, unknownCtx := rfc9484ConnectIPDatagramPayloadForPolicy(data, cfg.ConnectIPEchoContextIDs)
		if unknownCtx {
			if cfg.ConnectIPDatagramUnknownContext != nil {
				cfg.ConnectIPDatagramUnknownContext.Inc()
			}
			continue
		}
		if drop {
			if cfg.ConnectIPDatagramDrops != nil {
				cfg.ConnectIPDatagramDrops.Inc()
			}
			continue
		}

		if dst, proto, dport, ipOK := parseConnectIPDatagramDestination(payload); ipOK {
			if !auth.IsAllowed(acl, dst.String(), proto, dport) {
				if cfg.ConnectIPDatagramACLDenied != nil {
					cfg.ConnectIPDatagramACLDenied.Inc()
				}
				continue
			}
			if maybeRelayConnectIPv4UDP(ctx, str, cfg, payload) {
				continue
			}
			if maybeRelayConnectIPv4ICMP(ctx, str, cfg, payload) {
				continue
			}
		}

		if err := str.SendDatagram(data); err != nil {
			log.Printf("connect-ip stub: datagram echo send err=%v", err)
			return
		}
		if cfg.ConnectIPDatagramsSent != nil {
			cfg.ConnectIPDatagramsSent.Inc()
		}
	}
}
