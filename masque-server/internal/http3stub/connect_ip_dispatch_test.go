package http3stub

import (
	"errors"
	"testing"

	"afbuyers/masque-server/internal/rfc9484"
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
