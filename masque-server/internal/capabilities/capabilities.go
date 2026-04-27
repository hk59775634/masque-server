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
	// ConnectIPRouteAdvertPushCIDR: CONNECT_IP_ROUTE_ADV_CIDR; optional IPv4 CIDR for one outbound ROUTE_ADVERTISEMENT after 200 (ACL must cover start–end).
	ConnectIPRouteAdvertPushCIDR string
	// ConnectIPTunKernelForward: CONNECT_IP_TUN_FORWARD on a Linux masque host (per-session TUN bridge; SNAT/routing not in-process).
	ConnectIPTunKernelForward bool
	// ConnectIPTunLinkUp: CONNECT_IP_TUN_LINK_UP with TUN forward — server runs ip link set up after each TUN open.
	ConnectIPTunLinkUp bool
	// ConnectIPTunManagedNAT: CONNECT_IP_TUN_MANAGED_NAT with TUN forward — server applies minimal ip_forward/iptables automation.
	ConnectIPTunManagedNAT bool
	// ConnectIPTunShared: CONNECT_IP_TUN_SHARED with TUN forward — multiple streams share one TUN and demux by destination IP.
	ConnectIPTunShared bool
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
		if p.ConnectIPTunKernelForward {
			dgNote += " CONNECT_IP_TUN_FORWARD=1 (Linux): allowed Context ID 0 IP packets may be written to a per-session host TUN; replies from the TUN are sent to the client. Host sysctl/iptables SNAT and routing are operator-managed."
			if p.ConnectIPTunLinkUp {
				dgNote += " CONNECT_IP_TUN_LINK_UP: ip link set dev <tun> up after each successful TUN open (best-effort)."
			}
			if p.ConnectIPTunManagedNAT {
				dgNote += " CONNECT_IP_TUN_MANAGED_NAT: server applies minimal ip_forward/iptables MASQUERADE automation (requires operator egress interface config)."
			}
			if p.ConnectIPTunShared {
				dgNote += " CONNECT_IP_TUN_SHARED: streams share one host TUN with destination-IP demux."
			}
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
		if p.ConnectIPTunKernelForward {
			http3dg["tun_linux_per_session"] = true
			if p.ConnectIPTunLinkUp {
				http3dg["tun_linux_link_up"] = true
			}
			if p.ConnectIPTunManagedNAT {
				http3dg["tun_linux_managed_nat"] = true
			}
			if p.ConnectIPTunShared {
				http3dg["tun_linux_shared"] = true
			}
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
		connectIP := map[string]any{
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
				"decode":         []string{"ADDRESS_ASSIGN (0x01)", "ADDRESS_REQUEST (0x02)", "ROUTE_ADVERTISEMENT (0x03)"},
				"route_policy":   "each ROUTE_ADVERTISEMENT range must fall entirely inside one device ACL allow.cidr (same rule covers start and end); empty ACL allows all",
				"outbound_route": "optional: after 200 the server may write one ROUTE_ADVERTISEMENT when CONNECT_IP_ROUTE_ADV_CIDR is set and the inclusive IPv4 range fits ACL",
				"not_implemented": func() []string {
					out := []string{"CONNECT-IP TCP or IPv6 datagram relay"}
					if p.ConnectIPTunKernelForward {
						return append(out, "CONNECT-IP in-process SNAT or full carrier-grade routing")
					}
					return append(out, "CONNECT-IP kernel forward / full router")
				}(),
				"address_assign_reply": map[string]any{
					"stub": true,
					"note": "ADDRESS_REQUEST is answered with ADDRESS_ASSIGN using documentation addresses (192.0.2.1/32, 2001:db8::1/128) unless the client requests a specific IP that passes ACL",
				},
			},
			"note": func() string {
				s := "200 then RFC 9297 (SETTINGS datagrams) + RFC 9484 capsules; ROUTE vs ACL; stub ADDRESS_ASSIGN after ADDRESS_REQUEST"
				var tail []string
				if p.ConnectIPUDPRelayIPv4 {
					tail = append(tail, "CONNECT_IP_UDP_RELAY enables optional IPv4/UDP relay (see http3_datagrams)")
				}
				if p.ConnectIPTunKernelForward {
					tail = append(tail, "CONNECT_IP_TUN_FORWARD: Linux per-session TUN bridge (operator sysctl/iptables for SNAT)")
				}
				if len(tail) == 0 {
					return s + "; no server-side TUN"
				}
				return s + "; " + strings.Join(tail, "; ")
			}(),
			"dev": map[string]any{
				"skip_auth_env":       "CONNECT_IP_SKIP_AUTH or MASQUE_CONNECT_IP_SKIP_AUTH = 1|true|yes|on disables Bearer/Device-Fingerprint (not for production)",
				"echo_contexts_env":   "CONNECT_IP_STUB_ECHO_CONTEXTS=comma-separated non-zero Context IDs (e.g. 2,4) peels those datagrams for ACL/opaque echo instead of dropping as unknown (development only)",
				"udp_relay_env":       "CONNECT_IP_UDP_RELAY=1|true|yes|on enables optional IPv4/UDP user-space relay for Context ID 0 datagrams after ACL (not for production without review)",
				"icmp_relay_env":      "CONNECT_IP_ICMP_RELAY=1|true|yes|on enables optional IPv4 ICMP Echo relay (ping) after ACL; typically requires root or CAP_NET_RAW",
				"route_adv_push_env":  "CONNECT_IP_ROUTE_ADV_CIDR=<ipv4/cidr>: optional; server sends one ROUTE_ADVERTISEMENT after 200 when the inclusive range fits device ACL (same rule as inbound routes)",
				"tun_forward_env":     "CONNECT_IP_TUN_FORWARD=1|true|yes|on (Linux only): per-session TUN for ACL-allowed IP datagrams; CONNECT_IP_TUN_NAME optional (TUNSETIFF); requires /dev/net/tun (typically root). SNAT (e.g. iptables MASQUERADE) and ip_forward are not applied by masque-server.",
				"tun_link_up_env":     "CONNECT_IP_TUN_LINK_UP=1|true|yes|on (Linux, requires CONNECT_IP_TUN_FORWARD): after each successful TUN open, run ip link set dev <ifname> up (best-effort log on failure; needs ip(8) in PATH and CAP_NET_ADMIN).",
				"tun_managed_nat_env": "CONNECT_IP_TUN_MANAGED_NAT=1|true|yes|on (Linux, requires CONNECT_IP_TUN_FORWARD): apply net.ipv4.ip_forward=1 and iptables FORWARD/MASQUERADE rules. Requires CONNECT_IP_TUN_EGRESS_IFACE; optional CONNECT_IP_TUN_ADDR_CIDR for ip addr replace.",
				"tun_shared_env":      "CONNECT_IP_TUN_SHARED=1|true|yes|on (Linux, requires CONNECT_IP_TUN_FORWARD): share one host TUN across streams and demux by destination IP learned from inbound source IPs.",
				"tun_shared_ttl_env":  "CONNECT_IP_TUN_SHARED_BINDING_TTL=<duration> (default 5m): stale source-IP binding eviction window in shared TUN mode.",
			},
		}
		if p.ConnectIPRouteAdvertPushCIDR != "" {
			connectIP["route_push"] = map[string]any{
				"cidr": p.ConnectIPRouteAdvertPushCIDR,
				"note": "ROUTE_ADVERTISEMENT is written on the CONNECT stream right after hijack (skipped if outside ACL)",
			}
		}
		quicMap["connect_ip"] = connectIP
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
						n += "; CONNECT_IP_UDP_RELAY + CONNECT_IP_ICMP_RELAY: IPv4/UDP and ICMP Echo may be relayed"
					case p.ConnectIPUDPRelayIPv4:
						n += "; CONNECT_IP_UDP_RELAY on: IPv4/UDP may be relayed to the inner destination"
					case p.ConnectIPICMPRelayIPv4:
						n += "; CONNECT_IP_ICMP_RELAY on: IPv4 ICMP Echo may be relayed"
					default:
						n += "; default echo stub (set CONNECT_IP_UDP_RELAY / CONNECT_IP_ICMP_RELAY for user-space relay)"
					}
					if p.ConnectIPTunKernelForward {
						n += "; CONNECT_IP_TUN_FORWARD: optional Linux per-session TUN bridge"
						if p.ConnectIPTunManagedNAT {
							n += " + minimal managed NAT automation"
						}
						if p.ConnectIPTunShared {
							n += " + shared-TUN demux mode"
						}
					}
					return n
				}(),
			},
		},
	}
}
