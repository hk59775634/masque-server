package http3stub

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"afbuyers/masque-server/internal/capsules9297"
	"afbuyers/masque-server/internal/rfc9484"
	"github.com/quic-go/quic-go/http3"
)

const (
	connectIPProtocol      = "connect-ip"
	maxConnectIPDrainBytes = 4 << 20 // stub bound; not a production tunnel
)

func isConnectIPRequest(r *http.Request) bool {
	if r.Method != http.MethodConnect {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Proto), connectIPProtocol)
}

func incConnectIP(cfg ListenConfig, result string) {
	if cfg.ConnectIPResults == nil {
		return
	}
	cfg.ConnectIPResults.WithLabelValues(result).Inc()
}

func bearerDeviceToken(h http.Header) (tok string, ok bool) {
	v := h.Get("Authorization")
	const prefix = "Bearer "
	if len(v) <= len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) {
		return "", false
	}
	tok = strings.TrimSpace(v[len(prefix):])
	return tok, tok != ""
}

// serveConnectIPStub handles RFC 9484-style extended CONNECT (:protocol connect-ip).
// It returns 200 with Capsule-Protocol, hijacks the HTTP/3 stream, then parses RFC 9297 capsules.
// RFC 9484 ADDRESS_REQUEST / ROUTE_ADVERTISEMENT / ADDRESS_ASSIGN payloads are decoded;
// ROUTE_ADVERTISEMENT ranges must lie within a single device ACL "allow" CIDR.
// ADDRESS_REQUEST is answered with a stub ADDRESS_ASSIGN (192.0.2.1/32 or 2001:db8::1/128 when unspecified; preferred addresses must pass ACL).
// HTTP Datagrams (RFC 9297): RFC 9484 Context ID (varint) is peeled when present; unknown non-zero IDs drop unless CONNECT_IP_STUB_ECHO_CONTEXTS allowlist (dev); inner IPv4/IPv6 is ACL-checked then echoed unless CONNECT_IP_UDP_RELAY (IPv4/UDP) or CONNECT_IP_ICMP_RELAY (IPv4 ICMP Echo) handles the packet (user-space relay + reply). Opaque inner payloads are echoed (not kernel routing).
func serveConnectIPStub(w http.ResponseWriter, r *http.Request, cfg ListenConfig) {
	if r.URL == nil || r.URL.Scheme == "" || r.URL.Host == "" || r.URL.Path == "" {
		incConnectIP(cfg, "bad_request")
		http.Error(w, "extended CONNECT requires :scheme, :authority, :path", http.StatusBadRequest)
		return
	}

	var acl map[string]any
	if cfg.Authorizer != nil {
		tok, ok := bearerDeviceToken(r.Header)
		if !ok {
			incConnectIP(cfg, "unauthorized")
			http.Error(w, "missing or invalid Authorization: Bearer device_token", http.StatusUnauthorized)
			return
		}
		fp := strings.TrimSpace(r.Header.Get("Device-Fingerprint"))
		if fp == "" {
			incConnectIP(cfg, "unauthorized")
			http.Error(w, "missing Device-Fingerprint header", http.StatusUnauthorized)
			return
		}
		auth0 := time.Now()
		authz, err := cfg.Authorizer.Authorize(r.Context(), tok, fp)
		if cfg.AuthorizeLatencyObserve != nil {
			cfg.AuthorizeLatencyObserve.Observe(time.Since(auth0).Seconds())
		}
		if err != nil {
			incConnectIP(cfg, "unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		acl = authz.ACL
	}

	// RFC 9297 / MASQUE stacks negotiate capsule types on this stream.
	w.Header().Set("Capsule-Protocol", "?1")
	w.WriteHeader(http.StatusOK)

	hs, ok := w.(http3.HTTPStreamer)
	if !ok {
		incConnectIP(cfg, "internal_error")
		log.Printf("connect-ip stub: response writer does not support HTTPStream hijack")
		return
	}
	str := hs.HTTPStream()
	if g := cfg.ConnectIPStreamsActive; g != nil {
		g.Inc()
		defer g.Dec()
	}
	defer str.Close()

	maybePushConnectIPRouteAdvert(str, acl, cfg)

	dgCtx, dgCancel := context.WithCancel(str.Context())
	var dgWg sync.WaitGroup
	dgWg.Add(1)
	go func() {
		defer dgWg.Done()
		runConnectIPDatagramEchoLoop(dgCtx, str, cfg, acl)
	}()

	var st capsules9297.Stats
	derr := capsules9297.Pump(str, str, capsules9297.DrainOptions{
		MaxWireBytes:   maxConnectIPDrainBytes,
		MaxCapsuleBody: capsules9297.DefaultMaxCapsuleBody,
	}, &st, func(typ uint64, payload []byte) ([][]byte, error) {
		return handleConnectIPCapsule(typ, payload, acl, cfg)
	})

	dgCancel()
	dgWg.Wait()

	result := "ok"
	switch {
	case derr == nil:
		if cfg.ConnectIPCapsulesParsed != nil && st.Capsules > 0 {
			cfg.ConnectIPCapsulesParsed.Add(float64(st.Capsules))
		}
		log.Printf("connect-ip stub: remote=%s capsules=%d wire_bytes=%d by_type=%v",
			r.RemoteAddr, st.Capsules, st.WireBytes, st.ByType)
	case errors.Is(derr, errConnectIPPolicyDenied) || errors.Is(derr, errAddressRequestDenied):
		result = "forbidden"
		if cfg.ConnectIPCapsuleParseErrors != nil {
			cfg.ConnectIPCapsuleParseErrors.WithLabelValues("policy").Inc()
		}
		log.Printf("connect-ip stub: policy denied remote=%s err=%v", r.RemoteAddr, derr)
	case errors.Is(derr, capsules9297.ErrWireLimit):
		result = "drain_cap"
		log.Printf("connect-ip stub: wire limit remote=%s err=%v", r.RemoteAddr, derr)
	case errors.Is(derr, capsules9297.ErrCapsuleTooLarge):
		result = "capsule_error"
		if cfg.ConnectIPCapsuleParseErrors != nil {
			cfg.ConnectIPCapsuleParseErrors.WithLabelValues("too_large").Inc()
		}
		log.Printf("connect-ip stub: capsule too large remote=%s err=%v", r.RemoteAddr, derr)
	case errors.Is(derr, capsules9297.ErrTrailingBytes):
		result = "capsule_error"
		if cfg.ConnectIPCapsuleParseErrors != nil {
			cfg.ConnectIPCapsuleParseErrors.WithLabelValues("truncated").Inc()
		}
		log.Printf("connect-ip stub: truncated stream remote=%s err=%v", r.RemoteAddr, derr)
	case errors.Is(derr, rfc9484.ErrMalformed):
		result = "capsule_error"
		if cfg.ConnectIPCapsuleParseErrors != nil {
			cfg.ConnectIPCapsuleParseErrors.WithLabelValues("rfc9484").Inc()
		}
		log.Printf("connect-ip stub: rfc9484 parse remote=%s err=%v", r.RemoteAddr, derr)
	default:
		result = "read_error"
		log.Printf("connect-ip stub: drain err remote=%s err=%v", r.RemoteAddr, derr)
	}
	incConnectIP(cfg, result)
}
