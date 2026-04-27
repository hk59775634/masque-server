package http3stub

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"afbuyers/masque-server/internal/capabilities"
	"afbuyers/masque-server/internal/rfc9484"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/quic-go/quic-go/quicvarint"
)

func TestDispatchRoutePolicyDenied(t *testing.T) {
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "10.0.0.0/8", "protocol": "any", "port": "any"},
		},
	}
	var p []byte
	p = append(p, 4)
	p = append(p, 192, 0, 2, 1)
	p = append(p, 192, 0, 2, 2)
	p = append(p, 0)

	_, err := handleConnectIPCapsule(rfc9484.CapsuleRouteAdvertisement, p, acl, ListenConfig{})
	if !errors.Is(err, errConnectIPPolicyDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestDispatchRoutePolicyAllowed(t *testing.T) {
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "10.0.0.0/8", "protocol": "any", "port": "any"},
		},
	}
	var p []byte
	p = append(p, 4)
	p = append(p, 10, 1, 0, 1)
	p = append(p, 10, 1, 0, 9)
	p = append(p, 0)

	_, err := handleConnectIPCapsule(rfc9484.CapsuleRouteAdvertisement, p, acl, ListenConfig{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddressRequestReturnsAssignCapsule(t *testing.T) {
	var p []byte
	p = quicvarint.Append(p, 1)
	p = append(p, 4)
	p = append(p, make([]byte, 4)...)
	p = append(p, 32)

	replies, err := handleConnectIPCapsule(rfc9484.CapsuleAddressRequest, p, nil, ListenConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 || len(replies[0]) < 2 {
		t.Fatalf("replies=%v", replies)
	}
	typ, _, err := quicvarint.Parse(replies[0])
	if err != nil || typ != rfc9484.CapsuleAddressAssign {
		t.Fatalf("type=%d err=%v", typ, err)
	}
}

func TestMaybePushConnectIPRouteAdvertWritesCapsule(t *testing.T) {
	var buf bytes.Buffer
	routePush := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_connect_ip_route_push_total", Help: "test"}, []string{"result"})
	cfg := ListenConfig{
		Params:                    capabilities.Params{ConnectIPRouteAdvertPushCIDR: "10.0.0.0/24"},
		ConnectIPRoutePushResults: routePush,
	}
	maybePushConnectIPRouteAdvert(&buf, nil, cfg)
	data := buf.Bytes()
	if len(data) == 0 {
		t.Fatal("expected wire bytes")
	}
	vr := quicvarint.NewReader(bytes.NewReader(data))
	typ, err := quicvarint.Read(vr)
	if err != nil || typ != rfc9484.CapsuleRouteAdvertisement {
		t.Fatalf("capsule type=%d err=%v", typ, err)
	}
	ln, err := quicvarint.Read(vr)
	if err != nil {
		t.Fatal(err)
	}
	inner := make([]byte, ln)
	if _, err := io.ReadFull(vr, inner); err != nil {
		t.Fatal(err)
	}
	got, err := rfc9484.ParseRouteAdvertisement(inner)
	if err != nil || len(got) != 1 {
		t.Fatalf("parse: %v len=%d", err, len(got))
	}
	if n := testutil.ToFloat64(routePush.WithLabelValues("sent")); n != 1 {
		t.Fatalf("route_push sent=%v want 1", n)
	}
}

func TestMaybePushConnectIPRouteAdvertSkippedOutsideACL(t *testing.T) {
	var buf bytes.Buffer
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "192.168.0.0/16", "protocol": "any", "port": "any"},
		},
	}
	cfg := ListenConfig{
		Params:                    capabilities.Params{ConnectIPRouteAdvertPushCIDR: "10.0.0.0/8"},
		ConnectIPRoutePushResults: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_connect_ip_route_push_acl_total", Help: "test"}, []string{"result"}),
	}
	maybePushConnectIPRouteAdvert(&buf, acl, cfg)
	if buf.Len() != 0 {
		t.Fatalf("expected no write, got %d bytes", buf.Len())
	}
	if n := testutil.ToFloat64(cfg.ConnectIPRoutePushResults.WithLabelValues("acl_denied")); n != 1 {
		t.Fatalf("route_push acl_denied=%v want 1", n)
	}
}
