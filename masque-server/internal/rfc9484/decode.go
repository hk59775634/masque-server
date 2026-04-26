package rfc9484

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/quic-go/quic-go/quicvarint"
)

var (
	ErrMalformed = errors.New("rfc9484: malformed capsule payload")
)

// AssignedAddress is one entry inside ADDRESS_ASSIGN (RFC 9484 §4.7.1).
type AssignedAddress struct {
	RequestID    uint64
	IPVersion    uint8
	IP           net.IP
	PrefixLength uint8
}

// RequestedAddress is one entry inside ADDRESS_REQUEST (RFC 9484 §4.7.2).
type RequestedAddress struct {
	RequestID    uint64
	IPVersion    uint8
	IP           net.IP
	PrefixLength uint8
}

// IPRange is one entry inside ROUTE_ADVERTISEMENT (RFC 9484 §4.7.3).
type IPRange struct {
	IPVersion  uint8
	Start, End net.IP
	Protocol   uint8
}

// ParseAddressAssign decodes the payload of an ADDRESS_ASSIGN capsule (type 0x01).
func ParseAddressAssign(payload []byte) ([]AssignedAddress, error) {
	r := bytes.NewReader(payload)
	vr := quicvarint.NewReader(r)
	var out []AssignedAddress
	for r.Len() > 0 {
		reqID, err := quicvarint.Read(vr)
		if err != nil {
			return nil, fmt.Errorf("%w: request id: %v", ErrMalformed, err)
		}
		ipVer, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: ip version: %v", ErrMalformed, err)
		}
		if ipVer != 4 && ipVer != 6 {
			return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, ipVer)
		}
		addrLen := 4
		if ipVer == 6 {
			addrLen = 16
		}
		ipBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return nil, fmt.Errorf("%w: ip address: %v", ErrMalformed, err)
		}
		pfx, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: prefix length: %v", ErrMalformed, err)
		}
		if int(pfx) > addrLen*8 {
			return nil, fmt.Errorf("%w: prefix length %d", ErrMalformed, pfx)
		}
		out = append(out, AssignedAddress{
			RequestID:    reqID,
			IPVersion:    ipVer,
			IP:           net.IP(append(net.IP(nil), ipBuf...)),
			PrefixLength: pfx,
		})
	}
	return out, nil
}

// ParseAddressRequest decodes ADDRESS_REQUEST (0x02). Zero entries are invalid per RFC 9484.
func ParseAddressRequest(payload []byte) ([]RequestedAddress, error) {
	r := bytes.NewReader(payload)
	vr := quicvarint.NewReader(r)
	var out []RequestedAddress
	for r.Len() > 0 {
		reqID, err := quicvarint.Read(vr)
		if err != nil {
			return nil, fmt.Errorf("%w: request id: %v", ErrMalformed, err)
		}
		if reqID == 0 {
			return nil, fmt.Errorf("%w: request id zero", ErrMalformed)
		}
		ipVer, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: ip version: %v", ErrMalformed, err)
		}
		if ipVer != 4 && ipVer != 6 {
			return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, ipVer)
		}
		addrLen := 4
		if ipVer == 6 {
			addrLen = 16
		}
		ipBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return nil, fmt.Errorf("%w: ip address: %v", ErrMalformed, err)
		}
		pfx, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: prefix length: %v", ErrMalformed, err)
		}
		if int(pfx) > addrLen*8 {
			return nil, fmt.Errorf("%w: prefix length %d", ErrMalformed, pfx)
		}
		out = append(out, RequestedAddress{
			RequestID:    reqID,
			IPVersion:    ipVer,
			IP:           net.IP(append(net.IP(nil), ipBuf...)),
			PrefixLength: pfx,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: zero requested addresses", ErrMalformed)
	}
	return out, nil
}

// ParseRouteAdvertisement decodes ROUTE_ADVERTISEMENT (0x03).
func ParseRouteAdvertisement(payload []byte) ([]IPRange, error) {
	r := bytes.NewReader(payload)
	var out []IPRange
	for r.Len() > 0 {
		ipVer, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: ip version: %v", ErrMalformed, err)
		}
		if ipVer != 4 && ipVer != 6 {
			return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, ipVer)
		}
		addrLen := 4
		if ipVer == 6 {
			addrLen = 16
		}
		startBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, startBuf); err != nil {
			return nil, fmt.Errorf("%w: start ip: %v", ErrMalformed, err)
		}
		endBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, endBuf); err != nil {
			return nil, fmt.Errorf("%w: end ip: %v", ErrMalformed, err)
		}
		proto, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: protocol: %v", ErrMalformed, err)
		}
		start := net.IP(append(net.IP(nil), startBuf...))
		end := net.IP(append(net.IP(nil), endBuf...))
		if bytes.Compare(start, end) > 0 {
			return nil, fmt.Errorf("%w: start after end", ErrMalformed)
		}
		out = append(out, IPRange{IPVersion: ipVer, Start: start, End: end, Protocol: proto})
	}
	return out, nil
}
