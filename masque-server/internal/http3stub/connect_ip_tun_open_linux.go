//go:build linux

package http3stub

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// openConnectIPTunForward opens a new TUN device for one CONNECT-IP stream (IFF_TUN | IFF_NO_PI).
// requestedName may be empty for a kernel-assigned interface name.
func openConnectIPTunForward(requestedName string) (f *os.File, ifName string, cleanup func(), err error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR, 0)
	if err != nil {
		return nil, "", nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(strings.TrimSpace(requestedName))
	if err != nil {
		unix.Close(fd)
		return nil, "", nil, err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return nil, "", nil, fmt.Errorf("TUNSETIFF: %w", err)
	}
	ifName = ifr.Name()
	f = os.NewFile(uintptr(fd), ifName)
	cleanup = func() { _ = f.Close() }
	return f, ifName, cleanup, nil
}
