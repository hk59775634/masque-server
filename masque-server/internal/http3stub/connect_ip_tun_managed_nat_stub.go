//go:build !linux

package http3stub

func maybeConfigureConnectIPTunManagedNAT(ifName string, cfg ListenConfig) bool { return true }
