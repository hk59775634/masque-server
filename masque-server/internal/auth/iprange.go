package auth

import (
	"net"
)

// ACLCoversIPRange reports whether some single "allow" CIDR in acl contains both start and end
// (inclusive IPv4/IPv6 range). Used for ROUTE_ADVERTISEMENT policy checks; empty acl allows all.
func ACLCoversIPRange(acl map[string]any, start, end net.IP) bool {
	if len(acl) == 0 {
		return true
	}
	if start == nil || end == nil {
		return false
	}
	items, ok := acl["allow"].([]any)
	if !ok {
		return false
	}
	for _, raw := range items {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cidr, _ := entry["cidr"].(string)
		if cidr == "" || cidr == "0.0.0.0/0" || cidr == "::/0" {
			return true
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if n.Contains(start) && n.Contains(end) {
			return true
		}
	}
	return false
}
