package http3stub

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"afbuyers/masque-server/internal/auth"
	"github.com/quic-go/quic-go/http3"
)

// maxConnectIPDatagramBytes bounds echo and relay payload size (stub; not a production router).
const maxConnectIPDatagramBytes = 1200

// processInboundConnectIPDatagram handles one inbound HTTP datagram: metrics, RFC 9484 peel, ACL, optional
// IPv4 UDP/ICMP relay. If tun is nil, allowed packets are echoed. If tun is non-nil, allowed IP-shaped packets
// not handled by relay are written to the TUN device instead of echoed; non-IP inner payloads are still echoed.
func processInboundConnectIPDatagram(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any, data []byte, tun *os.File) error {
	if len(data) == 0 || len(data) > maxConnectIPDatagramBytes {
		if cfg.ConnectIPDatagramDrops != nil {
			cfg.ConnectIPDatagramDrops.Inc()
		}
		return nil
	}
	if cfg.ConnectIPDatagramsReceived != nil {
		cfg.ConnectIPDatagramsReceived.Inc()
	}

	payload, drop, unknownCtx := rfc9484ConnectIPDatagramPayloadForPolicy(data, cfg.ConnectIPEchoContextIDs)
	if unknownCtx {
		if cfg.ConnectIPDatagramUnknownContext != nil {
			cfg.ConnectIPDatagramUnknownContext.Inc()
		}
		return nil
	}
	if drop {
		if cfg.ConnectIPDatagramDrops != nil {
			cfg.ConnectIPDatagramDrops.Inc()
		}
		return nil
	}

	if dst, proto, dport, ipOK := parseConnectIPDatagramDestination(payload); ipOK {
		if !auth.IsAllowed(acl, dst.String(), proto, dport) {
			if cfg.ConnectIPDatagramACLDenied != nil {
				cfg.ConnectIPDatagramACLDenied.Inc()
			}
			return nil
		}
		if maybeRelayConnectIPv4UDP(ctx, str, cfg, payload) {
			return nil
		}
		if maybeRelayConnectIPv4ICMP(ctx, str, cfg, payload) {
			return nil
		}
		if tun != nil {
			if _, err := tun.Write(payload); err != nil {
				return fmt.Errorf("tun write: %w", err)
			}
			return nil
		}
	}

	if err := str.SendDatagram(data); err != nil {
		return fmt.Errorf("datagram send: %w", err)
	}
	if cfg.ConnectIPDatagramsSent != nil {
		cfg.ConnectIPDatagramsSent.Inc()
	}
	return nil
}

// runConnectIPTunDatagramLoop bridges RFC 9297 datagrams on str to a host TUN (one session, one TUN fd).
func runConnectIPTunDatagramLoop(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any, tun *os.File, tunName string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var shutdownOnce sync.Once
	shutdown := func() { shutdownOnce.Do(cancel) }

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 65536)
		for {
			if ctx.Err() != nil {
				return
			}
			n, err := tun.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("connect-ip tun: read %s: %v", tunName, err)
				shutdown()
				return
			}
			if n <= 0 {
				continue
			}
			if n > maxConnectIPDatagramBytes {
				if cfg.ConnectIPDatagramDrops != nil {
					cfg.ConnectIPDatagramDrops.Inc()
				}
				continue
			}
			frame := encodeRFC9484ContextZeroIPPacket(append([]byte(nil), buf[:n]...))
			if len(frame) > maxConnectIPDatagramBytes {
				if cfg.ConnectIPDatagramDrops != nil {
					cfg.ConnectIPDatagramDrops.Inc()
				}
				continue
			}
			if err := str.SendDatagram(frame); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("connect-ip tun: datagram send: %v", err)
				shutdown()
				return
			}
			if cfg.ConnectIPDatagramsSent != nil {
				cfg.ConnectIPDatagramsSent.Inc()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if ctx.Err() != nil {
				return
			}
			data, err := str.ReceiveDatagram(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("connect-ip tun: datagram recv end remote err=%v", err)
				shutdown()
				return
			}
			if err := processInboundConnectIPDatagram(ctx, str, cfg, acl, data, tun); err != nil {
				log.Printf("connect-ip tun: %v", err)
				shutdown()
				return
			}
		}
	}()

	wg.Wait()
}

// runConnectIPDatagramEchoLoop receives HTTP Datagrams (RFC 9297) on the CONNECT-IP stream.
// RFC 9484 Section 6: payloads may start with a Context ID (QUIC varint); ID 0 wraps a full IP packet.
// IP-shaped inner payloads use the same device ACL as POST /connect; optional CONNECT_IP_UDP_RELAY may
// forward unfragmented IPv4/UDP and return the reply instead of echoing; optional CONNECT_IP_ICMP_RELAY does the
// same for IPv4 ICMP Echo; opaque / legacy payloads are echoed.
// Optional ConnectIPTunForward (Linux): bridges allowed IP payloads to a per-session TUN instead of echoing.
func runConnectIPDatagramEchoLoop(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any) {
	if cfg.ConnectIPTunForward {
		tun, tunName, cleanup, err := openConnectIPTunForward(cfg.ConnectIPTunName)
		if err == nil {
			defer cleanup()
			if !maybeBringUpConnectIPTun(tunName, cfg.ConnectIPTunLinkUp) && cfg.ConnectIPTunLinkUpFailures != nil {
				cfg.ConnectIPTunLinkUpFailures.Inc()
			}
			if !maybeConfigureConnectIPTunManagedNAT(tunName, cfg) {
				if cfg.ConnectIPTunOpenEchoFallbacks != nil {
					cfg.ConnectIPTunOpenEchoFallbacks.Inc()
				}
				log.Printf("connect-ip: managed NAT unavailable on %s; echo mode", tunName)
				goto fallbackEcho
			}
			if cfg.ConnectIPTunBridgeActive != nil {
				cfg.ConnectIPTunBridgeActive.Inc()
				defer cfg.ConnectIPTunBridgeActive.Dec()
			}
			log.Printf("connect-ip: tun forward if=%s (per-session)", tunName)
			runConnectIPTunDatagramLoop(ctx, str, cfg, acl, tun, tunName)
			return
		}
		if cfg.ConnectIPTunOpenEchoFallbacks != nil {
			cfg.ConnectIPTunOpenEchoFallbacks.Inc()
		}
		log.Printf("connect-ip: tun forward unavailable: %v; echo mode", err)
	}

fallbackEcho:
	for {
		data, err := str.ReceiveDatagram(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("connect-ip stub: datagram recv end remote err=%v", err)
			return
		}
		if err := processInboundConnectIPDatagram(ctx, str, cfg, acl, data, nil); err != nil {
			log.Printf("connect-ip stub: %v", err)
			return
		}
	}
}
