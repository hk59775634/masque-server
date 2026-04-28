//go:build !linux

package http3stub

func maybeBringUpConnectIPTun(ifName string, enabled bool) bool { return true }

func maybeSetConnectIPTunMTU(ifName string) bool { return true }
