package capabilities

import "strings"

// Params drives the public JSON for GET /v1/masque/capabilities (TCP and HTTP/3 stubs).
type Params struct {
	Version                string
	TCPListenAddr          string
	ControlPlaneBaseURL    string // e.g. http://127.0.0.1:8000 (no trailing slash)
	QUICListenAddr         string // non-empty => HTTP/3 stub is configured
	MainListenerTLS        bool   // LISTEN_TLS_CERT + LISTEN_TLS_KEY set (HTTPS on main TCP listener)
	ConnectIPUDPRelayIPv4  bool   // CONNECT_IP_UDP_RELAY: optional IPv4/UDP user-space relay on CONNECT-IP
	ConnectIPICMPRelayIPv4 bool   // CONNECT_IP_ICMP_RELAY: optional IPv4 ICMP Echo relay on CONNECT-IP
}

// Build returns the capabilities document shared by TCP and QUIC listeners.
func Build(p Params) map[string]any {
	cp := strings.TrimRight(strings.TrimSpace(p.ControlPlaneBaseURL), "/")
	authorizeURL := cp + "/api/v1/server/authorize"

	quicMap := map[string]any{
		"rfc": "RFC 9298 (MASQUE over QUIC)",
	}
	if p.QUICListenAddr != "" {
		dgNote := "RFC 9484 Context ID (varint) prefix: ID 0 wraps raw IP; non-zero IDs dropped unless CONNECT_IP_STUB_ECHO_CONTEXTS allowlist (dev); non-IP inner echoed; IP inner uses POST /connect-style ACL (IPv6 walks common extension headers)"
		if p.ConnectIPUDPRelayIPv4 {
			dgNote += "; CONNECT_IP_UDP_RELAY=1: unfragmented IPv4/UDP is forwarded to the inner destination after ACL and the reply is sent as a new Context ID 0 datagram (no TCP/IPv6 relay; not kernel routing)."
		} else {
			dgNote += "; no kernel routing."
		}
		if p.ConnectIPICMPRelayIPv4 {
			dgNote += " CONNECT_IP_ICMP_RELAY=1: IPv4 ICMP Echo (ping) may be relayed after ACL (typically requires root / CAP_NET_RAW)."
		}
		http3dg := map[string]any{
			"settings": "RFC 9297 HTTP Datagrams negotiated (H3_DATAGRAM); QUIC datagram extension enabled on listener",
			"echo":     true,
			"note":     dgNote,
		}
		if p.ConnectIPUDPRelayIPv4 {
			http3dg["udp_ipv4_relay"] = true
		}
		if p.ConnectIPICMPRelayIPv4 {
			http3dg["icmp_ipv4_echo_relay"] = true
		}
		switch {
		case p.ConnectIPUDPRelayIPv4 && p.ConnectIPICMPRelayIPv4:
			http3dg["ip_forward"] = "ipv4_udp_icmp_user_space"
		case p.ConnectIPUDPRelayIPv4:
			http3dg["ip_forward"] = "ipv4_udp_user_space"
		case p.ConnectIPICMPRelayIPv4:
			http3dg["ip_forward"] = "ipv4_icmp_user_space"
		default:
			http3dg["ip_forward"] = false
		}

		quicMap["enabled"] = true
		quicMap["status"] = "http3_stub_connect_ip"
		quicMap["listen_udp_addr"] = p.QUICListenAddr
		quicMap["tls"] = map[string]any{"mode": "ephemeral_self_signed"}
		quicMap["connect_ip"] = map[string]any{
			"stub":            true,
			"rfc":             "RFC 9484 (CONNECT-IP); RFC 9220 (extended CONNECT); RFC 9297 (capsules)",
			"path_hint":       "any :path with extended CONNECT (e.g. /.well-known/masque/connect-ip)",
			"capsule_hdr":     "Capsule-Protocol: ?1 on 200",
			"http3_datagrams": http3dg,
			"auth": map[string]any{
				"headers": map[string]string{
					"Authorization":      "Bearer <device_token> (same token as POST /connect JSON)",
					"Device-Fingerprint": "<fingerprint>",
				},
				"note": "masque-server calls the same control-plane /api/v1/server/authorize as TCP routes before 200",
			},
			"rfc9484": map[string]any{
				"decode":          []string{"ADDRESS_ASSIGN (0x01)", "ADDRESS_REQUEST (0x02)", "ROUTE_ADVERTISEMENT (0x03)"},
				"route_policy":    "each ROUTE_ADVERTISEMENT range must fall entirely inside one device ACL allow.cidr (same rule covers start and end); empty ACL allows all",
				"not_implemented": []string{"CONNECT-IP TCP or IPv6 datagram relay", "CONNECT-IP kernel forward / full router"},
				"address_assign_reply": map[string]any{
					"stub": true,
					"note": "ADDRESS_REQUEST is answered with ADDRESS_ASSIGN using documentation addresses (192.0.2.1/32, 2001:db8::1/128) unless the client requests a specific IP that passes ACL",
				},
			},
			"note": func() string {
				s := "200 then RFC 9297 (SETTINGS datagrams) + RFC 9484 capsules; ROUTE vs ACL; stub ADDRESS_ASSIGN after ADDRESS_REQUEST"
				if p.ConnectIPUDPRelayIPv4 {
					return s + "; CONNECT_IP_UDP_RELAY enables optional IPv4/UDP relay (see http3_datagrams)"
				}
				return s + "; no server-side TUN"
			}(),
			"dev": map[string]any{
				"skip_auth_env":     "CONNECT_IP_SKIP_AUTH or MASQUE_CONNECT_IP_SKIP_AUTH = 1|true|yes|on disables Bearer/Device-Fingerprint (not for production)",
				"echo_contexts_env": "CONNECT_IP_STUB_ECHO_CONTEXTS=comma-separated non-zero Context IDs (e.g. 2,4) peels those datagrams for ACL/opaque echo instead of dropping as unknown (development only)",
				"udp_relay_env":     "CONNECT_IP_UDP_RELAY=1|true|yes|on enables optional IPv4/UDP user-space relay for Context ID 0 datagrams after ACL (not for production without review)",
				"icmp_relay_env":    "CONNECT_IP_ICMP_RELAY=1|true|yes|on enables optional IPv4 ICMP Echo relay (ping) after ACL; typically requires root or CAP_NET_RAW",
			},
		}
		quicMap["note"] = "UDP HTTP/3: /healthz, /v1/masque/capabilities, and extended CONNECT :protocol connect-ip (stub). Use curl --http3 -k for local checks; CONNECT-IP needs an HTTP/3 client with extended CONNECT."
	} else {
		quicMap["enabled"] = false
		quicMap["status"] = "planned"
		quicMap["note"] = "No UDP listener. Set QUIC_LISTEN_ADDR to enable the HTTP/3 health/capabilities stub."
	}

	transport := map[string]any{
		"listen_addr":   p.TCPListenAddr,
		"stack":         "tcp+http/1.1",
		"connect_route": "POST /connect",
	}
	if p.QUICListenAddr != "" {
		transport["http3_stub"] = map[string]any{
			"listen_udp": p.QUICListenAddr,
			"alpn":       []string{"h3"},
		}
	}
	if p.MainListenerTLS {
		transport["tls"] = map[string]any{
			"enabled": true,
			"mode":    "server_terminating",
			"note":    "LISTEN_TLS_CERT / LISTEN_TLS_KEY; clients must use https:// for API paths on this listener",
		}
	}

	return map[string]any{
		"service":   "masque-server",
		"version":   p.Version,
		"transport": transport,
		"tunnel": map[string]any{
			"mode":         "minimal-http-stub",
			"masque_ready": false,
			"planned_alpn": []string{"h3"},
			"control_plane": map[string]any{
				"authorize_url": authorizeURL,
			},
			"quic": quicMap,
			"phase2a": map[string]any{
				"tcp_server_probe": true,
				"tcp_probe_route":  "POST /v1/masque/tcp-probe",
				"note":             "masque dials TCP from server after control-plane authorize + ACL; not an end-user IP tunnel",
			},
			"phase2b": map[string]any{
				"connect_ip_quic_stub": p.QUICListenAddr != "",
				"note": func() string {
					n := "QUIC listener: CONNECT-IP with authorize + RFC 9484 capsules + RFC 9297 datagrams (RFC 9484 Context ID peel; IP ACL like POST /connect)"
					switch {
					case p.ConnectIPUDPRelayIPv4 && p.ConnectIPICMPRelayIPv4:
						return n + "; CONNECT_IP_UDP_RELAY + CONNECT_IP_ICMP_RELAY: IPv4/UDP and ICMP Echo may be relayed"
					case p.ConnectIPUDPRelayIPv4:
						return n + "; CONNECT_IP_UDP_RELAY on: IPv4/UDP may be relayed to the inner destination"
					case p.ConnectIPICMPRelayIPv4:
						return n + "; CONNECT_IP_ICMP_RELAY on: IPv4 ICMP Echo may be relayed"
					default:
						return n + "; default echo stub (set CONNECT_IP_UDP_RELAY / CONNECT_IP_ICMP_RELAY for user-space relay)"
					}
				}(),
			},
		},
	}
}
