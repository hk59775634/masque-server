package rfc9484

import (
	"fmt"
	"net"

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

// EncodeRouteAdvertisementPayload encodes RFC 9484 ROUTE_ADVERTISEMENT inner body (ordered ranges).
func EncodeRouteAdvertisementPayload(ranges []IPRange) ([]byte, error) {
	var out []byte
	for _, rg := range ranges {
		if rg.IPVersion != 4 && rg.IPVersion != 6 {
			return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, rg.IPVersion)
		}
		out = append(out, rg.IPVersion)
		switch rg.IPVersion {
		case 4:
			s, e := rg.Start.To4(), rg.End.To4()
			if s == nil || e == nil {
				return nil, fmt.Errorf("%w: invalid IPv4 range", ErrMalformed)
			}
			out = append(out, s...)
			out = append(out, e...)
		case 6:
			s, e := rg.Start.To16(), rg.End.To16()
			if s == nil || e == nil || len(s) != 16 || len(e) != 16 {
				return nil, fmt.Errorf("%w: invalid IPv6 range", ErrMalformed)
			}
			out = append(out, s...)
			out = append(out, e...)
		}
		out = append(out, rg.Protocol)
	}
	return out, nil
}

// EncodeRouteAdvertisement returns a full ROUTE_ADVERTISEMENT (0x03) capsule.
func EncodeRouteAdvertisement(ranges []IPRange) ([]byte, error) {
	inner, err := EncodeRouteAdvertisementPayload(ranges)
	if err != nil {
		return nil, err
	}
	return EncodeCapsule(CapsuleRouteAdvertisement, inner), nil
}

// IPv4CIDRToIPRange returns an inclusive IPv4 IPRange (protocol 0 = any) for a CIDR string.
func IPv4CIDRToIPRange(cidr string) (IPRange, error) {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return IPRange{}, err
	}
	v4 := ip.To4()
	if v4 == nil {
		return IPRange{}, fmt.Errorf("only IPv4 CIDR supported")
	}
	mask := n.Mask
	if len(mask) != 4 {
		return IPRange{}, fmt.Errorf("invalid IPv4 mask")
	}
	start := v4.Mask(mask).To4()
	if start == nil {
		return IPRange{}, fmt.Errorf("invalid network")
	}
	end := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		end[i] = start[i] | ^mask[i]
	}
	return IPRange{IPVersion: 4, Start: start, End: end, Protocol: 0}, nil
}
