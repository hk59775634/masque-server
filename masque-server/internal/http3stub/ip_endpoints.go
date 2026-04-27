package http3stub

import "net"

// parseConnectIPDatagramEndpoints returns source/destination IP for minimal IPv4/IPv6 packets.
func parseConnectIPDatagramEndpoints(b []byte) (src net.IP, dst net.IP, ok bool) {
	if len(b) < 1 {
		return nil, nil, false
	}
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return nil, nil, false
		}
		s := net.IPv4(b[12], b[13], b[14], b[15]).To4()
		d := net.IPv4(b[16], b[17], b[18], b[19]).To4()
		if s == nil || d == nil {
			return nil, nil, false
		}
		return s, d, true
	case 6:
		if len(b) < 40 {
			return nil, nil, false
		}
		s := make(net.IP, 16)
		d := make(net.IP, 16)
		copy(s, b[8:24])
		copy(d, b[24:40])
		return s, d, true
	default:
		return nil, nil, false
	}
}
