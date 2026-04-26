package rfc9484

import (
	"fmt"

	"github.com/quic-go/quic-go/quicvarint"
)

// EncodeAddressAssignPayload encodes the inner Assigned Address sequence (RFC 9484 Figure 7 body).
func EncodeAddressAssignPayload(addrs []AssignedAddress) ([]byte, error) {
	var out []byte
	for _, a := range addrs {
		out = quicvarint.Append(out, a.RequestID)
		out = append(out, a.IPVersion)
		switch a.IPVersion {
		case 4:
			ip4 := a.IP.To4()
			if ip4 == nil {
				return nil, fmt.Errorf("%w: invalid IPv4 in assign", ErrMalformed)
			}
			out = append(out, ip4...)
		case 6:
			ip6 := a.IP.To16()
			if ip6 == nil || len(ip6) != 16 {
				return nil, fmt.Errorf("%w: invalid IPv6 in assign", ErrMalformed)
			}
			out = append(out, ip6...)
		default:
			return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, a.IPVersion)
		}
		out = append(out, a.PrefixLength)
	}
	return out, nil
}

// EncodeCapsule wraps inner with RFC 9297 type/length/value.
func EncodeCapsule(capsuleType uint64, inner []byte) []byte {
	var out []byte
	out = quicvarint.Append(out, capsuleType)
	out = quicvarint.Append(out, uint64(len(inner)))
	out = append(out, inner...)
	return out
}

// EncodeAddressAssign returns a full ADDRESS_ASSIGN (0x01) capsule.
func EncodeAddressAssign(addrs []AssignedAddress) ([]byte, error) {
	inner, err := EncodeAddressAssignPayload(addrs)
	if err != nil {
		return nil, err
	}
	return EncodeCapsule(CapsuleAddressAssign, inner), nil
}
