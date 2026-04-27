package main

import (
	"fmt"

	"github.com/quic-go/quic-go/quicvarint"
)

// rfc9484PrependContext0 returns an HTTP/3 datagram payload: RFC 9484 §6 Context ID 0 (varint) + IP packet.
func rfc9484PrependContext0(ipPacket []byte) []byte {
	return append(quicvarint.Append(nil, 0), ipPacket...)
}

// rfc9484StripContext0 parses an incoming datagram; returns the inner IP packet when Context ID is 0.
func rfc9484StripContext0(dg []byte) ([]byte, error) {
	if len(dg) == 0 {
		return nil, fmt.Errorf("empty datagram")
	}
	cid, n, err := quicvarint.Parse(dg)
	if err != nil {
		return nil, fmt.Errorf("parse context id: %w", err)
	}
	if n <= 0 || n > len(dg) {
		return nil, fmt.Errorf("invalid context id prefix length %d", n)
	}
	if cid != 0 {
		return nil, fmt.Errorf("unsupported context id %d (only 0 for IP payload)", cid)
	}
	return dg[n:], nil
}
