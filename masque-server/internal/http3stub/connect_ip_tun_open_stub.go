//go:build !linux

package http3stub

import (
	"errors"
	"os"
)

// openConnectIPTunForward is only supported on Linux (CONNECT_IP_TUN_FORWARD).
func openConnectIPTunForward(requestedName string) (*os.File, string, func(), error) {
	return nil, "", nil, errors.New("CONNECT_IP_TUN_FORWARD requires Linux")
}
