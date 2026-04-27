package http3stub

import (
	"context"

	"afbuyers/masque-server/internal/auth"
	"afbuyers/masque-server/internal/capabilities"

	"github.com/prometheus/client_golang/prometheus"
)

// Authorizer matches *auth.Client for CONNECT-IP preflight (same contract as POST /connect).
type Authorizer interface {
	Authorize(ctx context.Context, deviceToken, fingerprint string) (*auth.AuthorizeResponse, error)
}

// ListenConfig configures the UDP HTTP/3 listener (health, capabilities, CONNECT-IP stub).
type ListenConfig struct {
	Params capabilities.Params

	// Authorizer, if non-nil, requires Authorization: Bearer <device_token> and
	// Device-Fingerprint: <fingerprint> on CONNECT-IP before 200.
	Authorizer Authorizer

	// ConnectIPResults, if non-nil, records one label per CONNECT-IP attempt (result=...).
	ConnectIPResults *prometheus.CounterVec

	// AuthorizeLatencyObserve, if non-nil, records control-plane authorize RTT for CONNECT-IP.
	AuthorizeLatencyObserve prometheus.Observer

	// ConnectIPCapsulesParsed, if non-nil, adds the number of RFC 9297 capsules parsed per stream.
	ConnectIPCapsulesParsed prometheus.Counter

	// ConnectIPCapsuleParseErrors, if non-nil, incremented once per CONNECT-IP stream when capsule parsing fails.
	ConnectIPCapsuleParseErrors *prometheus.CounterVec

	// RFC9484Capsules, if non-nil, counts decoded RFC 9484 capsule payloads by name (route_advertisement, address_request, address_assign).
	RFC9484Capsules *prometheus.CounterVec

	// ConnectIPAddressAssignWrites, if non-nil, counts ADDRESS_ASSIGN capsules written after ADDRESS_REQUEST.
	ConnectIPAddressAssignWrites prometheus.Counter

	// ConnectIPDatagramsReceived / Sent count HTTP Datagram payloads on CONNECT-IP streams (stub echo path).
	ConnectIPDatagramsReceived prometheus.Counter
	ConnectIPDatagramsSent     prometheus.Counter

	// ConnectIPDatagramDrops, if non-nil, counts inbound datagrams dropped (empty or over size bound).
	ConnectIPDatagramDrops prometheus.Counter

	// ConnectIPDatagramACLDenied, if non-nil, counts inbound datagrams that looked like IPv4/IPv6 but failed device ACL (no echo).
	ConnectIPDatagramACLDenied prometheus.Counter

	// ConnectIPDatagramUnknownContext, if non-nil, counts RFC 9484 datagrams with non-zero Context ID (unregistered in stub).
	ConnectIPDatagramUnknownContext prometheus.Counter

	// ConnectIPEchoContextIDs, if non-nil, lists non-zero RFC 9484 Context IDs accepted for peel + inner handling (CONNECT_IP_STUB_ECHO_CONTEXTS; dev only).
	ConnectIPEchoContextIDs map[uint64]struct{}

	// ConnectIPUDPRelay enables CONNECT_IP_UDP_RELAY: unfragmented IPv4/UDP (Context ID 0) is forwarded to the inner
	// destination after ACL; the reply is sent as a new datagram instead of echoing the request (IPv6/TCP unchanged).
	ConnectIPUDPRelay bool

	// ConnectIPUDPRelayReplies, if non-nil, counts IPv4/UDP relay replies successfully sent.
	ConnectIPUDPRelayReplies prometheus.Counter

	// ConnectIPUDPRelayErrors, if non-nil, counts IPv4/UDP relay failures (labels: dial, read, parse, build, send).
	ConnectIPUDPRelayErrors *prometheus.CounterVec

	// ConnectIPICMPRelay enables CONNECT_IP_ICMP_RELAY: IPv4 ICMP Echo Request (type 8) is forwarded after ACL
	// and Echo Reply is returned as a new Context ID 0 datagram (requires raw ICMP socket / typically root).
	ConnectIPICMPRelay bool

	// ConnectIPICMPRelayReplies, if non-nil, counts successful ICMP echo relay replies sent.
	ConnectIPICMPRelayReplies prometheus.Counter

	// ConnectIPICMPRelayErrors, if non-nil, counts ICMP relay failures (labels: dial, read, parse, build, send, mismatch).
	ConnectIPICMPRelayErrors *prometheus.CounterVec

	// ConnectIPStreamsActive, if non-nil, gauge incremented after 200 + stream hijack until handler returns.
	ConnectIPStreamsActive prometheus.Gauge

	// ConnectIPRoutePushResults, if non-nil, counts proactive ROUTE_ADVERTISEMENT push outcomes
	// (result labels: sent, invalid_cidr, acl_denied, encode_error, write_error).
	ConnectIPRoutePushResults *prometheus.CounterVec

	// ConnectIPTunForward (Linux only): after ACL, IPv4/UDP and IPv4 ICMP relay unchanged; other IP-shaped
	// Context ID 0 payloads are written to a per-session host TUN instead of echoed. Packets read from the TUN
	// are sent to the client as RFC 9484 Context ID 0 datagrams. Requires /dev/net/tun (typically root or
	// CAP_NET_ADMIN). SNAT and default routing are not configured by masque-server — see CONNECT_IP_TUN_FORWARD docs.
	ConnectIPTunForward bool
	// ConnectIPTunName is passed to TUNSETIFF (may be empty for kernel-assigned name).
	ConnectIPTunName string

	// ConnectIPTunBridgeActive, if non-nil, gauge incremented while a CONNECT-IP stream runs the host TUN bridge loop.
	ConnectIPTunBridgeActive prometheus.Gauge

	// ConnectIPTunOpenEchoFallbacks, if non-nil, counts CONNECT_IP_TUN_FORWARD sessions where opening /dev/net/tun failed and the handler fell back to echo mode.
	ConnectIPTunOpenEchoFallbacks prometheus.Counter

	// ConnectIPTunLinkUp (Linux): after a successful TUN open, run `ip link set dev <if> up` (CONNECT_IP_TUN_LINK_UP).
	// Requires `ip` in PATH and typically CAP_NET_ADMIN in addition to TUN access.
	ConnectIPTunLinkUp bool
}
