package auth

import (
	"net"
	"strconv"
	"strings"
)

func IsAllowed(acl map[string]any, destinationIP string, protocol string, port int) bool {
	if len(acl) == 0 {
		return true
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
		if matchCIDR(entry["cidr"], destinationIP) && matchProtocol(entry["protocol"], protocol) && matchPort(entry["port"], port) {
			return true
		}
	}

	return false
}

func matchCIDR(cidrRaw any, destinationIP string) bool {
	cidr, _ := cidrRaw.(string)
	if cidr == "" || cidr == "0.0.0.0/0" {
		return true
	}
	ip := net.ParseIP(destinationIP)
	_, n, err := net.ParseCIDR(cidr)
	return err == nil && ip != nil && n.Contains(ip)
}

func matchProtocol(protocolRaw any, protocol string) bool {
	rule, _ := protocolRaw.(string)
	rule = strings.ToLower(rule)
	protocol = strings.ToLower(protocol)
	return rule == "" || rule == "any" || rule == protocol
}

func matchPort(portRaw any, port int) bool {
	switch v := portRaw.(type) {
	case string:
		if v == "" || v == "any" {
			return true
		}
		p, err := strconv.Atoi(v)
		return err == nil && p == port
	case float64:
		return int(v) == port
	default:
		return true
	}
}
