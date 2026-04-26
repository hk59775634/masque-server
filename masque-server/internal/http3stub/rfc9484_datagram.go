package http3stub

import (
	"github.com/quic-go/quic-go/quicvarint"
)

// rfc9484ConnectIPDatagramPayloadForPolicy returns the inner bytes used for IP parsing and ACL.
// Per RFC 9484 Section 6, CONNECT-IP HTTP Datagram payloads begin with a Context ID (varint).
// Context ID 0 carries a full IP packet; other IDs require registration (RFC 9484). The stub drops unknown
// non-zero IDs unless echoContextAllowlist contains the ID (CONNECT_IP_STUB_ECHO_CONTEXTS; development only).
// If the leading bytes are not a valid QUIC varint, b is returned unchanged (legacy / probe payloads).
//
// drop means the datagram must not be echoed (unknown non-zero context, or Context ID 0 with no payload).
func rfc9484ConnectIPDatagramPayloadForPolicy(b []byte, echoContextAllowlist map[uint64]struct{}) (payload []byte, drop bool, unknownNonZeroContext bool) {
	if len(b) == 0 {
		return b, false, false
	}
	cid, n, err := quicvarint.Parse(b)
	if err != nil {
		return b, false, false
	}
	if cid != 0 {
		if echoContextAllowlist != nil {
			if _, ok := echoContextAllowlist[cid]; ok {
				if n >= len(b) {
					return nil, true, false
				}
				return b[n:], false, false
			}
		}
		return nil, true, true
	}
	if n >= len(b) {
		return nil, true, false
	}
	return b[n:], false, false
}

// encodeRFC9484ContextZeroIPPacket wraps a raw IP packet as RFC 9484 Context ID 0 (for tests / clients).
func encodeRFC9484ContextZeroIPPacket(ipPacket []byte) []byte {
	out := quicvarint.Append(nil, 0)
	return append(out, ipPacket...)
}
