//go:build !linux

package http3stub

// EnsureConnectIPSharedTunReady is a no-op on non-Linux platforms.
func EnsureConnectIPSharedTunReady(cfg ListenConfig) error {
	_ = cfg
	return nil
}

