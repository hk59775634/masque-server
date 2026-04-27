package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/quic-go/quic-go/quicvarint"
)

// RFC 9297 capsule type IDs used by RFC 9484 (same as masque-server internal/rfc9484).
const (
	capsuleAddressAssign  uint64 = 0x01
	capsuleAddressRequest uint64 = 0x02
	capsuleRouteAdvertise uint64 = 0x03
)

var errCapsuleNeedMore = errors.New("need more capsule data")

func encodeCapsule9297(capsuleType uint64, inner []byte) []byte {
	var out []byte
	out = quicvarint.Append(out, capsuleType)
	out = quicvarint.Append(out, uint64(len(inner)))
	return append(out, inner...)
}

// encodeAddressRequestIPv4Unspecified returns one ADDRESS_REQUEST (0x02) capsule:
// request_id, IPv4, 0.0.0.0, prefix /32 (server stub may answer with ADDRESS_ASSIGN e.g. 192.0.2.1/32).
func encodeAddressRequestIPv4Unspecified(requestID uint64) []byte {
	var inner []byte
	inner = quicvarint.Append(inner, requestID)
	inner = append(inner, 4)
	inner = append(inner, make([]byte, 4)...)
	inner = append(inner, 32)
	return encodeCapsule9297(capsuleAddressRequest, inner)
}

func parseOneCapsule9297(b []byte, maxBody uint64) (typ uint64, payload []byte, consumed int, err error) {
	if len(b) == 0 {
		return 0, nil, 0, errCapsuleNeedMore
	}
	typ, n1, err := quicvarint.Parse(b)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, 0, errCapsuleNeedMore
		}
		return 0, nil, 0, err
	}
	if n1 > len(b) {
		return 0, nil, 0, errCapsuleNeedMore
	}
	off := n1
	if off >= len(b) {
		return 0, nil, 0, errCapsuleNeedMore
	}
	bodyLen, n2, err := quicvarint.Parse(b[off:])
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, 0, errCapsuleNeedMore
		}
		return 0, nil, 0, err
	}
	off += n2
	if bodyLen > maxBody {
		return 0, nil, 0, fmt.Errorf("capsule body too large: %d", bodyLen)
	}
	if uint64(len(b)-off) < bodyLen {
		return 0, nil, 0, errCapsuleNeedMore
	}
	end := off + int(bodyLen)
	return typ, b[off:end], end, nil
}

// assignedAddr is one entry from ADDRESS_ASSIGN (RFC 9484 §4.7.1).
type assignedAddr struct {
	RequestID    uint64
	IPVersion    uint8
	IP           net.IP
	PrefixLength uint8
}

type routeRange struct {
	IPVersion uint8
	Start     net.IP
	End       net.IP
	Protocol  uint8
}

// parseAddressAssignPayload decodes RFC 9484 ADDRESS_ASSIGN inner payload (Figure 7).
func parseAddressAssignPayload(payload []byte) ([]assignedAddr, error) {
	r := bytes.NewReader(payload)
	vr := quicvarint.NewReader(r)
	var out []assignedAddr
	for r.Len() > 0 {
		reqID, err := quicvarint.Read(vr)
		if err != nil {
			return nil, fmt.Errorf("request id: %w", err)
		}
		ipVer, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("ip version: %w", err)
		}
		if ipVer != 4 && ipVer != 6 {
			return nil, fmt.Errorf("ip version %d", ipVer)
		}
		addrLen := 4
		if ipVer == 6 {
			addrLen = 16
		}
		ipBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return nil, fmt.Errorf("ip: %w", err)
		}
		pfx, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("prefix: %w", err)
		}
		out = append(out, assignedAddr{
			RequestID:    reqID,
			IPVersion:    ipVer,
			IP:           net.IP(append(net.IP(nil), ipBuf...)),
			PrefixLength: pfx,
		})
	}
	return out, nil
}

// parseRouteAdvertisementPayload decodes RFC 9484 ROUTE_ADVERTISEMENT inner payload.
func parseRouteAdvertisementPayload(payload []byte) ([]routeRange, error) {
	r := bytes.NewReader(payload)
	var out []routeRange
	for r.Len() > 0 {
		ipVer, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("ip version: %w", err)
		}
		if ipVer != 4 && ipVer != 6 {
			return nil, fmt.Errorf("ip version %d", ipVer)
		}
		addrLen := 4
		if ipVer == 6 {
			addrLen = 16
		}
		startBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, startBuf); err != nil {
			return nil, fmt.Errorf("start ip: %w", err)
		}
		endBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(r, endBuf); err != nil {
			return nil, fmt.Errorf("end ip: %w", err)
		}
		proto, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("protocol: %w", err)
		}
		start := net.IP(append(net.IP(nil), startBuf...))
		end := net.IP(append(net.IP(nil), endBuf...))
		if bytes.Compare(start, end) > 0 {
			return nil, fmt.Errorf("start after end")
		}
		out = append(out, routeRange{
			IPVersion: ipVer,
			Start:     start,
			End:       end,
			Protocol:  proto,
		})
	}
	return out, nil
}

// routeRangeToCIDR converts an inclusive [start,end] range into a single CIDR if exact; false otherwise.
func routeRangeToCIDR(start, end net.IP) (string, bool) {
	s4 := start.To4()
	e4 := end.To4()
	if s4 == nil || e4 == nil {
		return "", false
	}
	a := binaryBigEndianU32(s4)
	b := binaryBigEndianU32(e4)
	if b < a {
		return "", false
	}
	size := b - a + 1
	if size == 0 || (size&(size-1)) != 0 {
		return "", false
	}
	if a&(size-1) != 0 {
		return "", false
	}
	pfx := 32
	for size > 1 {
		pfx--
		size >>= 1
	}
	return fmt.Sprintf("%s/%d", start.String(), pfx), true
}

func binaryBigEndianU32(ip net.IP) uint32 {
	v4 := ip.To4()
	if v4 == nil {
		return 0
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
}

// drainConnectIPCapsules reads the CONNECT-IP response stream until EOF. When wantAutoAddr is true,
// applies the first IPv4 ADDRESS_ASSIGN via `ip addr add`. assignNotify receives one CIDR when applied (optional).
func drainConnectIPCapsules(r io.Reader, ifName string, wantAutoAddr, wantAutoRoute bool, assignNotify chan<- string) error {
	const maxWire = 4 << 20
	const maxBody = 1 << 20
	buf := make([]byte, 32<<10)
	var carry []byte
	var applied bool
	var wire int64

	for {
		if wire >= maxWire {
			return fmt.Errorf("capsule stream exceeds %d bytes", maxWire)
		}
		n, err := r.Read(buf)
		wire += int64(n)
		carry = append(carry, buf[:n]...)

		for {
			typ, payload, consumed, perr := parseOneCapsule9297(carry, maxBody)
			if errors.Is(perr, errCapsuleNeedMore) {
				break
			}
			if perr != nil {
				log.Printf("connect-ip-tun: capsule parse error: %v", perr)
				return perr
			}
			carry = carry[consumed:]

			switch typ {
			case capsuleAddressAssign:
				if wantAutoAddr && !applied {
					addrs, aerr := parseAddressAssignPayload(payload)
					if aerr != nil {
						log.Printf("connect-ip-tun: ADDRESS_ASSIGN decode: %v", aerr)
					} else {
						for _, a := range addrs {
							if a.IPVersion == 4 && a.IP.To4() != nil {
								cidr := fmt.Sprintf("%s/%d", a.IP.String(), a.PrefixLength)
								cmd := exec.Command("ip", "addr", "add", cidr, "dev", ifName)
								cmd.Stderr = os.Stderr
								cmd.Stdout = os.Stdout
								if err := cmd.Run(); err != nil {
									log.Printf("connect-ip-tun: warn: ip addr add %s dev %s: %v", cidr, ifName, err)
								} else {
									log.Printf("connect-ip-tun: applied ADDRESS_ASSIGN %s on %s", cidr, ifName)
									if assignNotify != nil {
										select {
										case assignNotify <- cidr:
										default:
										}
									}
								}
								applied = true
								break
							}
						}
					}
				}
			case capsuleRouteAdvertise:
				if wantAutoRoute {
					ranges, rerr := parseRouteAdvertisementPayload(payload)
					if rerr != nil {
						log.Printf("connect-ip-tun: ROUTE_ADVERTISEMENT decode: %v", rerr)
						break
					}
					for _, rg := range ranges {
						// For now apply only IPv4 any-protocol routes representable as one CIDR.
						if rg.IPVersion != 4 || rg.Protocol != 0 {
							continue
						}
						cidr, ok := routeRangeToCIDR(rg.Start, rg.End)
						if !ok {
							log.Printf("connect-ip-tun: skip route range %s-%s (not single CIDR)", rg.Start, rg.End)
							continue
						}
						cmd := exec.Command("ip", "route", "replace", cidr, "dev", ifName)
						cmd.Stderr = os.Stderr
						cmd.Stdout = os.Stdout
						if err := cmd.Run(); err != nil {
							log.Printf("connect-ip-tun: warn: ip route replace %s dev %s: %v", cidr, ifName, err)
						} else {
							log.Printf("connect-ip-tun: applied ROUTE_ADVERTISEMENT %s on %s", cidr, ifName)
						}
					}
				}
			default:
				// ignore
			}
		}

		if err == io.EOF {
			if len(carry) > 0 {
				return fmt.Errorf("trailing %d bytes after capsules", len(carry))
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// connectIPCapsuleRequestID picks a non-zero request id for ADDRESS_REQUEST.
func connectIPCapsuleRequestID() uint64 {
	return uint64(time.Now().UnixNano())
}
