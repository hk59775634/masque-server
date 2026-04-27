package http3stub

import (
	"errors"
	"log"
	"net"
	"strings"

	"afbuyers/masque-server/internal/auth"
	"afbuyers/masque-server/internal/rfc9484"
)

var (
	errConnectIPPolicyDenied = errors.New("connect-ip: ROUTE_ADVERTISEMENT not covered by device ACL")
	errAddressRequestDenied  = errors.New("connect-ip: ADDRESS_REQUEST preference not allowed by device ACL")
)

func incRFC9484(cfg ListenConfig, capsule string) {
	if cfg.RFC9484Capsules != nil {
		cfg.RFC9484Capsules.WithLabelValues(capsule).Inc()
	}
}

func incRoutePush(cfg ListenConfig, result string) {
	if cfg.ConnectIPRoutePushResults != nil {
		cfg.ConnectIPRoutePushResults.WithLabelValues(result).Inc()
	}
}

type connectIPStreamWriter interface {
	Write(p []byte) (int, error)
}

// maybePushConnectIPRouteAdvert writes one ROUTE_ADVERTISEMENT after 200 when
// Params.ConnectIPRouteAdvertPushCIDR is set, the CIDR parses, and (if acl is non-empty) the range fits ACL.
func maybePushConnectIPRouteAdvert(str connectIPStreamWriter, acl map[string]any, cfg ListenConfig) {
	cidr := strings.TrimSpace(cfg.Params.ConnectIPRouteAdvertPushCIDR)
	if cidr == "" {
		return
	}
	rg, err := rfc9484.IPv4CIDRToIPRange(cidr)
	if err != nil {
		incRoutePush(cfg, "invalid_cidr")
		log.Printf("connect-ip stub: CONNECT_IP_ROUTE_ADV_CIDR invalid %q: %v", cidr, err)
		return
	}
	if !auth.ACLCoversIPRange(acl, rg.Start, rg.End) {
		incRoutePush(cfg, "acl_denied")
		log.Printf("connect-ip stub: skip route push %s: range outside device ACL", cidr)
		return
	}
	wire, err := rfc9484.EncodeRouteAdvertisement([]rfc9484.IPRange{rg})
	if err != nil {
		incRoutePush(cfg, "encode_error")
		log.Printf("connect-ip stub: encode route push: %v", err)
		return
	}
	if _, err := str.Write(wire); err != nil {
		incRoutePush(cfg, "write_error")
		log.Printf("connect-ip stub: route push write: %v", err)
		return
	}
	incRoutePush(cfg, "sent")
	incRFC9484(cfg, "route_advertisement_push")
}

// stubAssignFromRequest maps one requested address to a stub assignment (TEST-NET / documentation space).
// Non-zero requested IPs must fall inside a single allow.cidr (same rule as ROUTE).
func stubAssignFromRequest(q rfc9484.RequestedAddress, acl map[string]any) (rfc9484.AssignedAddress, error) {
	switch q.IPVersion {
	case 4:
		pfx := q.PrefixLength
		if pfx == 0 {
			pfx = 32
		}
		if int(pfx) > 32 {
			pfx = 32
		}
		ip := net.ParseIP("192.0.2.1").To4()
		if !q.IP.IsUnspecified() {
			v4 := q.IP.To4()
			if v4 == nil {
				return rfc9484.AssignedAddress{}, rfc9484.ErrMalformed
			}
			if !auth.ACLCoversIPRange(acl, v4, v4) {
				return rfc9484.AssignedAddress{}, errAddressRequestDenied
			}
			ip = v4
		}
		return rfc9484.AssignedAddress{
			RequestID:    q.RequestID,
			IPVersion:    4,
			IP:           ip,
			PrefixLength: pfx,
		}, nil

	case 6:
		pfx := q.PrefixLength
		if pfx == 0 {
			pfx = 128
		}
		if int(pfx) > 128 {
			pfx = 128
		}
		ip := net.ParseIP("2001:db8::1").To16()
		if !q.IP.IsUnspecified() {
			v6 := q.IP.To16()
			if v6 == nil {
				return rfc9484.AssignedAddress{}, rfc9484.ErrMalformed
			}
			if !auth.ACLCoversIPRange(acl, v6, v6) {
				return rfc9484.AssignedAddress{}, errAddressRequestDenied
			}
			ip = v6
		}
		return rfc9484.AssignedAddress{
			RequestID:    q.RequestID,
			IPVersion:    6,
			IP:           ip,
			PrefixLength: uint8(pfx),
		}, nil

	default:
		return rfc9484.AssignedAddress{}, rfc9484.ErrMalformed
	}
}

// handleConnectIPCapsule decodes RFC 9484 payloads, enforces ROUTE/ADDRESS policy, and may return
// ADDRESS_ASSIGN capsule bytes to write back on the CONNECT stream.
func handleConnectIPCapsule(typ uint64, payload []byte, acl map[string]any, cfg ListenConfig) ([][]byte, error) {
	switch typ {
	case rfc9484.CapsuleRouteAdvertisement:
		ranges, err := rfc9484.ParseRouteAdvertisement(payload)
		if err != nil {
			return nil, err
		}
		for _, rg := range ranges {
			if !auth.ACLCoversIPRange(acl, rg.Start, rg.End) {
				return nil, errConnectIPPolicyDenied
			}
		}
		incRFC9484(cfg, "route_advertisement")
		return nil, nil

	case rfc9484.CapsuleAddressRequest:
		reqs, err := rfc9484.ParseAddressRequest(payload)
		if err != nil {
			return nil, err
		}
		assigns := make([]rfc9484.AssignedAddress, 0, len(reqs))
		for _, q := range reqs {
			a, err := stubAssignFromRequest(q, acl)
			if err != nil {
				return nil, err
			}
			assigns = append(assigns, a)
		}
		wire, err := rfc9484.EncodeAddressAssign(assigns)
		if err != nil {
			return nil, err
		}
		incRFC9484(cfg, "address_request")
		if cfg.ConnectIPAddressAssignWrites != nil {
			cfg.ConnectIPAddressAssignWrites.Inc()
		}
		return [][]byte{wire}, nil

	case rfc9484.CapsuleAddressAssign:
		if _, err := rfc9484.ParseAddressAssign(payload); err != nil {
			return nil, err
		}
		incRFC9484(cfg, "address_assign")
		return nil, nil

	default:
		return nil, nil
	}
}
