package auth

import (
	"net"
	"testing"
)

func TestACLCoversIPRange(t *testing.T) {
	acl := map[string]any{
		"allow": []any{
			map[string]any{"cidr": "10.0.0.0/8", "protocol": "any", "port": "any"},
		},
	}
	s := net.ParseIP("10.1.0.1")
	e := net.ParseIP("10.1.0.9")
	if !ACLCoversIPRange(acl, s, e) {
		t.Fatal("expected allow")
	}
	if ACLCoversIPRange(acl, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2")) {
		t.Fatal("expected deny")
	}
}
