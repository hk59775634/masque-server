package http3stub

import (
	"strconv"
	"strings"
)

const maxEchoContextIDs = 32

// ParseConnectIPEchoContextAllowlist parses CONNECT_IP_STUB_ECHO_CONTEXTS (comma-separated decimal QUIC varints).
// Non-zero context IDs listed here are not treated as unknown: the varint is stripped and the remainder is passed
// to IP ACL / opaque echo logic (development only; not a substitute for proper Context registration).
func ParseConnectIPEchoContextAllowlist(s string) map[uint64]struct{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := make(map[uint64]struct{})
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseUint(part, 10, 62)
		if err != nil || v == 0 {
			continue
		}
		out[v] = struct{}{}
		if len(out) >= maxEchoContextIDs {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
