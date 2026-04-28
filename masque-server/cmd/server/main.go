package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"afbuyers/masque-server/internal/auth"
	"afbuyers/masque-server/internal/capabilities"
	"afbuyers/masque-server/internal/http3stub"
	"afbuyers/masque-server/internal/requestid"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const maxConnectBodyBytes = 65536

// Set at link time: -X main.version=... -X main.commit=... -X main.date=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type tunnelRequest struct {
	DeviceToken     string `json:"device_token"`
	Fingerprint     string `json:"fingerprint"`
	DestinationIP   string `json:"destination_ip"`
	DestinationPort int    `json:"destination_port"`
	Protocol        string `json:"protocol"`
}

type tcpProbeRequest struct {
	DeviceToken string `json:"device_token"`
	Fingerprint string `json:"fingerprint"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("masque-server %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built:  %s\n", date)
		fmt.Printf("go:     %s\n", runtime.Version())
		return
	}

	cpURL := strings.TrimRight(strings.TrimSpace(envOr("CONTROL_PLANE_URL", "http://127.0.0.1:8000")), "/")
	listenAddr := envOr("LISTEN_ADDR", ":8443")
	quicListen := strings.TrimSpace(os.Getenv("QUIC_LISTEN_ADDR"))
	tlsCertPath := strings.TrimSpace(os.Getenv("LISTEN_TLS_CERT"))
	tlsKeyPath := strings.TrimSpace(os.Getenv("LISTEN_TLS_KEY"))
	mainTLS := tlsCertPath != "" && tlsKeyPath != ""
	metricsRegistry := prometheus.NewRegistry()
	metrics := newServerMetrics(metricsRegistry)

	cpClient := auth.NewClient(cpURL)
	connectIPUDPRelay := isTruthyEnv("CONNECT_IP_UDP_RELAY")
	connectIPICMPRelay := isTruthyEnv("CONNECT_IP_ICMP_RELAY")
	connectIPRouteAdvCIDR := strings.TrimSpace(os.Getenv("CONNECT_IP_ROUTE_ADV_CIDR"))
	connectIPTunForward := false
	if isTruthyEnv("CONNECT_IP_TUN_FORWARD") {
		if runtime.GOOS == "linux" {
			connectIPTunForward = true
		} else {
			log.Printf("CONNECT_IP_TUN_FORWARD is set but ignored (GOOS=%s)", runtime.GOOS)
		}
	}
	connectIPTunName := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_NAME"))
	connectIPTunLinkUp := isTruthyEnv("CONNECT_IP_TUN_LINK_UP") && connectIPTunForward
	connectIPTunManagedNAT := isTruthyEnv("CONNECT_IP_TUN_MANAGED_NAT") && connectIPTunForward
	connectIPTunShared := connectIPTunForward
	if rawShared := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_SHARED")); rawShared != "" {
		if b, ok := parseBoolEnvValue(rawShared); ok {
			connectIPTunShared = b && connectIPTunForward
		} else {
			log.Printf("CONNECT_IP_TUN_SHARED=%q invalid; using default=%v", rawShared, connectIPTunShared)
		}
	}
	connectIPTunSharedBindingTTL := 5 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_SHARED_BINDING_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			connectIPTunSharedBindingTTL = d
		} else {
			log.Printf("CONNECT_IP_TUN_SHARED_BINDING_TTL parse failed (%q): %v; using %s", raw, err, connectIPTunSharedBindingTTL)
		}
	}
	connectIPTunEgressIF := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_EGRESS_IFACE"))
	connectIPTunAddrCIDR := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_ADDR_CIDR"))
	connectIPTunManagedNATBackend := "nftables"
	if raw := strings.TrimSpace(strings.ToLower(os.Getenv("CONNECT_IP_TUN_NAT_BACKEND"))); raw != "" {
		switch raw {
		case "nft", "nftables":
			connectIPTunManagedNATBackend = "nftables"
		case "iptables":
			connectIPTunManagedNATBackend = "iptables"
		default:
			log.Printf("CONNECT_IP_TUN_NAT_BACKEND=%q invalid; using nftables", raw)
		}
	}
	connectIPTunManagedNATFallback := true
	if rawFallback := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_NAT_FALLBACK_IPTABLES")); rawFallback != "" {
		if b, ok := parseBoolEnvValue(rawFallback); ok {
			connectIPTunManagedNATFallback = b
		} else {
			log.Printf("CONNECT_IP_TUN_NAT_FALLBACK_IPTABLES=%q invalid; using default=true", rawFallback)
		}
	}
	if connectIPTunManagedNAT && connectIPTunEgressIF == "" {
		log.Printf("CONNECT_IP_TUN_MANAGED_NAT is enabled but CONNECT_IP_TUN_EGRESS_IFACE is empty; disabling managed NAT to avoid repeated apply failures")
		connectIPTunManagedNAT = false
	}
	capParams := capabilities.Params{
		Version:                      version,
		TCPListenAddr:                listenAddr,
		ControlPlaneBaseURL:          cpURL,
		QUICListenAddr:               quicListen,
		MainListenerTLS:              mainTLS,
		ConnectIPUDPRelayIPv4:        connectIPUDPRelay,
		ConnectIPICMPRelayIPv4:       connectIPICMPRelay,
		ConnectIPRouteAdvertPushCIDR: connectIPRouteAdvCIDR,
		ConnectIPTunKernelForward:    connectIPTunForward,
		ConnectIPTunLinkUp:           connectIPTunLinkUp,
		ConnectIPTunManagedNAT:       connectIPTunManagedNAT,
		ConnectIPTunManagedNATBackend: connectIPTunManagedNATBackend,
		ConnectIPTunShared:           connectIPTunShared,
	}

	router := chi.NewRouter()
	router.Use(requestid.Middleware)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		metrics.healthChecksTotal.Inc()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": version,
			"ts":      time.Now().UTC(),
		})
	})
	router.Handle("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))

	router.Get("/v1/masque/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, capabilities.Build(capParams))
	})

	router.Post("/connect", func(w http.ResponseWriter, r *http.Request) {
		rid := requestid.FromContext(r.Context())
		start := time.Now()
		metrics.connectRequestsTotal.Inc()

		r.Body = http.MaxBytesReader(w, r.Body, maxConnectBodyBytes)

		var req tunnelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			reason := "bad_payload"
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				reason = "payload_too_large"
			}
			metrics.connectFailuresTotal.WithLabelValues(reason).Inc()
			log.Printf("connect rid=%s result=%s err=%v", rid, reason, err)
			if reason == "payload_too_large" {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}

		authStart := time.Now()
		authz, err := cpClient.Authorize(r.Context(), req.DeviceToken, req.Fingerprint)
		metrics.authorizeLatency.Observe(time.Since(authStart).Seconds())
		if err != nil {
			metrics.connectFailuresTotal.WithLabelValues("unauthorized").Inc()
			log.Printf("connect rid=%s result=unauthorized err=%v", rid, err)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		if req.DestinationIP != "" && !auth.IsAllowed(authz.ACL, req.DestinationIP, req.Protocol, req.DestinationPort) {
			metrics.connectFailuresTotal.WithLabelValues("acl_denied").Inc()
			log.Printf("connect rid=%s result=acl_denied device_id=%d dst=%s", rid, authz.DeviceID, req.DestinationIP)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "acl denied"})
			return
		}
		metrics.connectSuccessTotal.Inc()
		metrics.connectLatency.Observe(time.Since(start).Seconds())

		sid, err := randomSessionID()
		if err != nil {
			log.Printf("session id: %v, using fallback", err)
			sid = fmt.Sprintf("msq_fallback_%d", time.Now().UnixNano())
		}

		log.Printf("connect rid=%s result=ok device_id=%d user_id=%d session=%s elapsed=%s",
			rid, authz.DeviceID, authz.UserID, sid, time.Since(start).Truncate(time.Millisecond))

		writeJSON(w, http.StatusOK, map[string]any{
			"session":      sid,
			"user_id":      authz.UserID,
			"device_id":    authz.DeviceID,
			"policy_acl":   authz.ACL,
			"routes":       authz.Routes,
			"dns":          authz.DNS,
			"masque_ready": true,
		})
	})

	router.Post("/v1/masque/tcp-probe", func(w http.ResponseWriter, r *http.Request) {
		rid := requestid.FromContext(r.Context())
		r.Body = http.MaxBytesReader(w, r.Body, maxConnectBodyBytes)

		var req tcpProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			metrics.tcpProbeTotal.WithLabelValues("bad_payload").Inc()
			log.Printf("tcp_probe rid=%s result=bad_payload err=%v", rid, err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		host := strings.TrimSpace(req.Host)
		ip := net.ParseIP(host)
		if ip == nil || req.Port < 1 || req.Port > 65535 {
			metrics.tcpProbeTotal.WithLabelValues("bad_payload").Inc()
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "host must be a literal IP (v4 or v6); port must be 1-65535",
			})
			return
		}

		authStart := time.Now()
		authz, err := cpClient.Authorize(r.Context(), req.DeviceToken, req.Fingerprint)
		metrics.authorizeLatency.Observe(time.Since(authStart).Seconds())
		if err != nil {
			metrics.tcpProbeTotal.WithLabelValues("unauthorized").Inc()
			log.Printf("tcp_probe rid=%s result=unauthorized err=%v", rid, err)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		dest := ip.String()
		if !auth.IsAllowed(authz.ACL, dest, "tcp", req.Port) {
			metrics.tcpProbeTotal.WithLabelValues("acl_denied").Inc()
			log.Printf("tcp_probe rid=%s result=acl_denied dst=%s:%d", rid, dest, req.Port)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "acl denied"})
			return
		}

		addr := net.JoinHostPort(dest, strconv.Itoa(req.Port))
		d0 := time.Now()
		dialer := net.Dialer{Timeout: 4 * time.Second}
		conn, err := dialer.DialContext(r.Context(), "tcp", addr)
		elapsed := time.Since(d0).Seconds()
		metrics.tcpProbeDialLatency.Observe(elapsed)
		if err != nil {
			metrics.tcpProbeTotal.WithLabelValues("dial_failed").Inc()
			log.Printf("tcp_probe rid=%s result=dial_failed dst=%s err=%v", rid, addr, err)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"ok": false, "error": "dial_failed", "remote": addr, "detail": err.Error(),
			})
			return
		}
		_ = conn.Close()
		metrics.tcpProbeTotal.WithLabelValues("ok").Inc()
		log.Printf("tcp_probe rid=%s result=ok dst=%s rtt_ms=%.2f", rid, addr, elapsed*1000)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"remote":    addr,
			"rtt_ms":    elapsed * 1000,
			"device_id": authz.DeviceID,
		})
	})

	if quicListen != "" {
		go func() {
			skipConnectIPAuth := isTruthyEnv("CONNECT_IP_SKIP_AUTH") || isTruthyEnv("MASQUE_CONNECT_IP_SKIP_AUTH")
			cfg := http3stub.ListenConfig{
				Params:                                  capParams,
				ConnectIPResults:                        metrics.connectIPTotal,
				AuthorizeLatencyObserve:                 metrics.authorizeLatency,
				ConnectIPCapsulesParsed:                 metrics.connectIPCapsulesParsed,
				ConnectIPCapsuleParseErrors:             metrics.connectIPCapsuleParseErrors,
				RFC9484Capsules:                         metrics.connectIPRFC9484Capsules,
				ConnectIPAddressAssignWrites:            metrics.connectIPAddressAssignWrites,
				ConnectIPDatagramsReceived:              metrics.connectIPDatagramsReceived,
				ConnectIPDatagramsSent:                  metrics.connectIPDatagramsSent,
				ConnectIPDatagramDrops:                  metrics.connectIPDatagramDrops,
				ConnectIPDatagramACLDenied:              metrics.connectIPDatagramACLDenied,
				ConnectIPDatagramUnknownContext:         metrics.connectIPDatagramUnknownContext,
				ConnectIPStreamsActive:                  metrics.connectIPStreamsActive,
				ConnectIPRoutePushResults:               metrics.connectIPRoutePushResults,
				ConnectIPUDPRelay:                       connectIPUDPRelay,
				ConnectIPUDPRelayReplies:                metrics.connectIPUDPRelayReplies,
				ConnectIPUDPRelayErrors:                 metrics.connectIPUDPRelayErrors,
				ConnectIPICMPRelay:                      connectIPICMPRelay,
				ConnectIPICMPRelayReplies:               metrics.connectIPICMPRelayReplies,
				ConnectIPICMPRelayErrors:                metrics.connectIPICMPRelayErrors,
				ConnectIPTunForward:                     connectIPTunForward,
				ConnectIPTunShared:                      connectIPTunShared,
				ConnectIPTunName:                        connectIPTunName,
				ConnectIPTunBridgeActive:                metrics.connectIPTunBridgeActive,
				ConnectIPTunOpenEchoFallbacks:           metrics.connectIPTunOpenEchoFallbacks,
				ConnectIPTunLinkUp:                      connectIPTunLinkUp,
				ConnectIPTunLinkUpFailures:              metrics.connectIPTunLinkUpFailures,
				ConnectIPTunManagedNAT:                  connectIPTunManagedNAT,
				ConnectIPTunManagedNATBackend:           connectIPTunManagedNATBackend,
				ConnectIPTunManagedNATAllowIPTablesFallback: connectIPTunManagedNATFallback,
				ConnectIPTunEgressInterface:             connectIPTunEgressIF,
				ConnectIPTunAddressCIDR:                 connectIPTunAddrCIDR,
				ConnectIPTunManagedNATApplyResults:      metrics.connectIPTunManagedNATApply,
				ConnectIPTunManagedNATBackendResults:    metrics.connectIPTunManagedNATBackend,
				ConnectIPTunSharedBindingConflicts:      metrics.connectIPTunSharedConflicts,
				ConnectIPTunSharedBindingConflictReasons: metrics.connectIPTunSharedConflictReason,
				ConnectIPTunSharedBindingStaleEvictions: metrics.connectIPTunSharedStaleEvictions,
				ConnectIPTunSharedBindingTTL:            connectIPTunSharedBindingTTL,
			}
			if !skipConnectIPAuth {
				cfg.Authorizer = cpClient
			} else {
				log.Printf("WARNING: CONNECT_IP_SKIP_AUTH is set; QUIC CONNECT-IP does not require device auth (development only)")
			}
			if echoCtx := http3stub.ParseConnectIPEchoContextAllowlist(os.Getenv("CONNECT_IP_STUB_ECHO_CONTEXTS")); echoCtx != nil {
				cfg.ConnectIPEchoContextIDs = echoCtx
				log.Printf("CONNECT_IP_STUB_ECHO_CONTEXTS: %d non-zero RFC 9484 context ID(s) accepted for peel (development only)", len(echoCtx))
			}
			if connectIPUDPRelay {
				log.Printf("CONNECT_IP_UDP_RELAY is set: IPv4/UDP CONNECT-IP datagrams may be forwarded after ACL (user-space relay; not a full router)")
			}
			if connectIPICMPRelay {
				log.Printf("CONNECT_IP_ICMP_RELAY is set: IPv4 ICMP Echo may be relayed after ACL (typically requires root/CAP_NET_RAW)")
			}
			if connectIPRouteAdvCIDR != "" {
				log.Printf("CONNECT_IP_ROUTE_ADV_CIDR=%s: server may push ROUTE_ADVERTISEMENT after 200 when ACL covers the range", connectIPRouteAdvCIDR)
			}
			if connectIPTunForward {
				log.Printf("CONNECT_IP_TUN_FORWARD: per-session host TUN bridge (CONNECT_IP_TUN_NAME=%q); host routing/SNAT remains operator-managed unless CONNECT_IP_TUN_MANAGED_NAT is enabled", connectIPTunName)
			}
			if connectIPTunLinkUp {
				log.Printf("CONNECT_IP_TUN_LINK_UP: will run `ip link set dev <tun> up` after each successful TUN open (requires ip(8) and typically CAP_NET_ADMIN)")
			}
			if connectIPTunManagedNAT {
				log.Printf("CONNECT_IP_TUN_MANAGED_NAT: will apply ip_forward/NAT automation for each session TUN (backend=%s fallback_iptables=%v egress=%q tun_addr=%q)", connectIPTunManagedNATBackend, connectIPTunManagedNATFallback, connectIPTunEgressIF, connectIPTunAddrCIDR)
			}
			if connectIPTunShared {
				log.Printf("CONNECT_IP_TUN_SHARED: all CONNECT-IP streams share one host TUN; return packets are demuxed by destination IP (binding_ttl=%s)", connectIPTunSharedBindingTTL)
			}
			if err := http3stub.EnsureConnectIPSharedTunReady(cfg); err != nil {
				log.Printf("CONNECT_IP_TUN_SHARED eager init failed: %v", err)
			} else if connectIPTunForward && connectIPTunShared {
				log.Printf("CONNECT_IP_TUN_SHARED eager init OK (interface pre-created before first session)")
			}
			if err := http3stub.Listen(cfg); err != nil {
				log.Printf("QUIC HTTP/3 stub: %v", err)
			}
		}()
	}

	if mainTLS {
		log.Printf("masque-server listening on %s (HTTPS), control-plane=%s", listenAddr, cpURL)
		if err := http.ListenAndServeTLS(listenAddr, tlsCertPath, tlsKeyPath, router); err != nil {
			log.Fatalf("server stopped: %v", err)
		}
		return
	}

	log.Printf("masque-server listening on %s, control-plane=%s", listenAddr, cpURL)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func envOr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func isTruthyEnv(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseBoolEnvValue(v string) (bool, bool) {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func randomSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "msq_" + hex.EncodeToString(b[:]), nil
}

type serverMetrics struct {
	connectRequestsTotal             prometheus.Counter
	connectSuccessTotal              prometheus.Counter
	connectFailuresTotal             *prometheus.CounterVec
	connectLatency                   prometheus.Histogram
	authorizeLatency                 prometheus.Histogram
	tcpProbeTotal                    *prometheus.CounterVec
	tcpProbeDialLatency              prometheus.Histogram
	connectIPTotal                   *prometheus.CounterVec
	connectIPCapsulesParsed          prometheus.Counter
	connectIPCapsuleParseErrors      *prometheus.CounterVec
	connectIPRFC9484Capsules         *prometheus.CounterVec
	connectIPAddressAssignWrites     prometheus.Counter
	connectIPDatagramsReceived       prometheus.Counter
	connectIPDatagramsSent           prometheus.Counter
	connectIPDatagramDrops           prometheus.Counter
	connectIPDatagramACLDenied       prometheus.Counter
	connectIPDatagramUnknownContext  prometheus.Counter
	connectIPStreamsActive           prometheus.Gauge
	connectIPRoutePushResults        *prometheus.CounterVec
	connectIPUDPRelayReplies         prometheus.Counter
	connectIPUDPRelayErrors          *prometheus.CounterVec
	connectIPICMPRelayReplies        prometheus.Counter
	connectIPICMPRelayErrors         *prometheus.CounterVec
	connectIPTunBridgeActive         prometheus.Gauge
	connectIPTunOpenEchoFallbacks    prometheus.Counter
	connectIPTunLinkUpFailures       prometheus.Counter
	connectIPTunManagedNATApply      *prometheus.CounterVec
	connectIPTunManagedNATBackend    *prometheus.CounterVec
	connectIPTunSharedConflicts      prometheus.Counter
	connectIPTunSharedConflictReason *prometheus.CounterVec
	connectIPTunSharedStaleEvictions prometheus.Counter
	healthChecksTotal                prometheus.Counter
}

func newServerMetrics(registry *prometheus.Registry) *serverMetrics {
	m := &serverMetrics{
		connectRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_requests_total",
			Help: "Total connect API requests.",
		}, []string{}).WithLabelValues(),
		connectSuccessTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_success_total",
			Help: "Total successful connect API requests.",
		}),
		connectFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_failures_total",
			Help: "Total failed connect API requests by reason.",
		}, []string{"reason"}),
		connectLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "masque_connect_latency_seconds",
			Help:    "Latency of connect API handler.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.3, 0.5, 1, 2, 5},
		}),
		authorizeLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "masque_authorize_latency_seconds",
			Help:    "Latency of control-plane POST /api/v1/server/authorize (connect, tcp-probe, CONNECT-IP).",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}),
		tcpProbeTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_tcp_probe_total",
			Help: "TCP probe outcomes (POST /v1/masque/tcp-probe).",
		}, []string{"result"}),
		tcpProbeDialLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "masque_tcp_probe_dial_latency_seconds",
			Help:    "Time to complete TCP dial during tcp-probe.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}),
		connectIPTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_requests_total",
			Help: "CONNECT-IP stub outcomes on the QUIC listener (result label).",
		}, []string{"result"}),
		connectIPCapsulesParsed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_capsules_parsed_total",
			Help: "RFC 9297 capsules successfully parsed on CONNECT-IP stub streams.",
		}),
		connectIPCapsuleParseErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_capsule_parse_errors_total",
			Help: "CONNECT-IP stub capsule parse failures (cause label).",
		}, []string{"cause"}),
		connectIPRFC9484Capsules: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_rfc9484_capsules_total",
			Help: "RFC 9484 capsule events on CONNECT-IP streams (decoded peer payloads, or server-pushed ROUTE_ADVERTISEMENT).",
		}, []string{"capsule"}),
		connectIPAddressAssignWrites: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_address_assign_writes_total",
			Help: "ADDRESS_ASSIGN capsules written on CONNECT-IP streams after ADDRESS_REQUEST (stub).",
		}),
		connectIPDatagramsReceived: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_datagrams_received_total",
			Help: "HTTP Datagram (RFC 9297) payloads received on CONNECT-IP streams (stub echo path).",
		}),
		connectIPDatagramsSent: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_datagrams_sent_total",
			Help: "HTTP Datagram payloads sent on CONNECT-IP streams (stub echo replies).",
		}),
		connectIPDatagramDrops: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_datagrams_dropped_total",
			Help: "Inbound HTTP Datagrams dropped on CONNECT-IP (empty, over size bound, or RFC 9484 Context ID 0 with no payload).",
		}),
		connectIPDatagramACLDenied: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_datagram_acl_denied_total",
			Help: "Inbound HTTP Datagrams parsed as IP but denied by device ACL (no echo).",
		}),
		connectIPDatagramUnknownContext: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_datagram_unknown_context_total",
			Help: "Inbound HTTP Datagrams with non-zero RFC 9484 Context ID (stub does not register contexts).",
		}),
		connectIPStreamsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "masque_connect_ip_streams_active",
			Help: "CONNECT-IP stub streams currently in the post-200 hijacked phase (same scope as capsule/datagram handlers).",
		}),
		connectIPRoutePushResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_route_push_total",
			Help: "Proactive CONNECT-IP ROUTE_ADVERTISEMENT outcomes when CONNECT_IP_ROUTE_ADV_CIDR is configured.",
		}, []string{"result"}),
		connectIPUDPRelayReplies: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_udp_relay_replies_total",
			Help: "CONNECT-IP IPv4/UDP relay replies sent (CONNECT_IP_UDP_RELAY).",
		}),
		connectIPUDPRelayErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_udp_relay_errors_total",
			Help: "CONNECT-IP IPv4/UDP relay failures (CONNECT_IP_UDP_RELAY).",
		}, []string{"reason"}),
		connectIPICMPRelayReplies: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_icmp_relay_replies_total",
			Help: "CONNECT-IP IPv4 ICMP Echo relay replies sent (CONNECT_IP_ICMP_RELAY).",
		}),
		connectIPICMPRelayErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_icmp_relay_errors_total",
			Help: "CONNECT-IP IPv4 ICMP Echo relay failures (CONNECT_IP_ICMP_RELAY).",
		}, []string{"reason"}),
		connectIPTunBridgeActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "masque_connect_ip_tun_bridge_active",
			Help: "CONNECT-IP streams currently bridging datagrams to a host TUN (CONNECT_IP_TUN_FORWARD on Linux).",
		}),
		connectIPTunOpenEchoFallbacks: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_open_echo_fallback_total",
			Help: "CONNECT_IP_TUN_FORWARD was enabled but opening the per-session TUN failed; stream used echo mode instead.",
		}),
		connectIPTunLinkUpFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_link_up_failures_total",
			Help: "CONNECT_IP_TUN_LINK_UP attempted to run `ip link set up` but failed.",
		}),
		connectIPTunManagedNATApply: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_managed_nat_apply_total",
			Help: "CONNECT_IP_TUN_MANAGED_NAT apply outcomes (result label).",
		}, []string{"result"}),
		connectIPTunManagedNATBackend: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_managed_nat_backend_total",
			Help: "CONNECT_IP_TUN_MANAGED_NAT backend outcomes by backend and result.",
		}, []string{"backend", "result"}),
		connectIPTunSharedConflicts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_shared_binding_conflicts_total",
			Help: "Shared TUN source-IP binding ownership changes across sessions.",
		}),
		connectIPTunSharedConflictReason: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_shared_binding_conflict_reasons_total",
			Help: "Shared TUN source-IP binding conflict reasons.",
		}, []string{"reason"}),
		connectIPTunSharedStaleEvictions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_ip_tun_shared_binding_stale_evictions_total",
			Help: "Shared TUN source-IP bindings evicted by TTL.",
		}),
		healthChecksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_healthz_requests_total",
			Help: "Total health endpoint hits.",
		}),
	}

	registry.MustRegister(
		m.connectRequestsTotal,
		m.connectSuccessTotal,
		m.connectFailuresTotal,
		m.connectLatency,
		m.authorizeLatency,
		m.tcpProbeTotal,
		m.tcpProbeDialLatency,
		m.connectIPTotal,
		m.connectIPCapsulesParsed,
		m.connectIPCapsuleParseErrors,
		m.connectIPRFC9484Capsules,
		m.connectIPAddressAssignWrites,
		m.connectIPDatagramsReceived,
		m.connectIPDatagramsSent,
		m.connectIPDatagramDrops,
		m.connectIPDatagramACLDenied,
		m.connectIPDatagramUnknownContext,
		m.connectIPStreamsActive,
		m.connectIPRoutePushResults,
		m.connectIPUDPRelayReplies,
		m.connectIPUDPRelayErrors,
		m.connectIPICMPRelayReplies,
		m.connectIPICMPRelayErrors,
		m.connectIPTunBridgeActive,
		m.connectIPTunOpenEchoFallbacks,
		m.connectIPTunLinkUpFailures,
		m.connectIPTunManagedNATApply,
		m.connectIPTunManagedNATBackend,
		m.connectIPTunSharedConflicts,
		m.connectIPTunSharedConflictReason,
		m.connectIPTunSharedStaleEvictions,
		m.healthChecksTotal,
	)

	// Pre-initialize label series so dashboards/bench checks can assert existence before first event.
	m.connectIPTunManagedNATApply.WithLabelValues("ok")
	m.connectIPTunManagedNATApply.WithLabelValues("error")
	m.connectIPTunManagedNATBackend.WithLabelValues("nftables", "ok")
	m.connectIPTunManagedNATBackend.WithLabelValues("nftables", "error")
	m.connectIPTunManagedNATBackend.WithLabelValues("nftables", "fallback")
	m.connectIPTunManagedNATBackend.WithLabelValues("iptables", "ok")
	m.connectIPTunManagedNATBackend.WithLabelValues("iptables", "error")
	m.connectIPTunManagedNATBackend.WithLabelValues("iptables", "fallback")
	m.connectIPTunSharedConflictReason.WithLabelValues("active_reassign")
	m.connectIPTunSharedConflictReason.WithLabelValues("stale_reassign")

	return m
}
