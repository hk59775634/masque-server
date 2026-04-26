// Package rfc9484 implements minimal CONNECT-IP (RFC 9484) capsule decoding.
package rfc9484

// HTTP capsule type IDs (IANA "HTTP Capsule Types", RFC 9484).
const (
	CapsuleDatagram           = 0x00 // RFC 9297 (not decoded here)
	CapsuleAddressAssign      = 0x01
	CapsuleAddressRequest     = 0x02
	CapsuleRouteAdvertisement = 0x03
)
