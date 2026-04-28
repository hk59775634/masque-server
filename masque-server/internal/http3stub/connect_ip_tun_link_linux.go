//go:build linux

package http3stub

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// maybeBringUpConnectIPTun runs `ip link set dev <if> up` when enabled (CONNECT_IP_TUN_LINK_UP).
// Returns true on success/no-op and false when the attempted action failed.
func maybeBringUpConnectIPTun(ifName string, enabled bool) bool {
	if !enabled || strings.TrimSpace(ifName) == "" {
		return true
	}
	if _, err := exec.LookPath("ip"); err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_LINK_UP: ip not in PATH: %v", err)
		return false
	}
	out, err := exec.Command("ip", "link", "set", "dev", ifName, "up").CombinedOutput()
	if err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_LINK_UP: ip link set dev %s up: %v: %s", ifName, err, strings.TrimSpace(string(out)))
		return false
	}
	return true
}

// maybeSetConnectIPTunMTU runs `ip link set dev <if> mtu <N>` when CONNECT_IP_TUN_MTU is set (e.g. 1280).
// Empty or "0" skips (kernel default, often 1500 on tun). Returns true on success or no-op.
func maybeSetConnectIPTunMTU(ifName string) bool {
	s := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_MTU"))
	if s == "" || s == "0" || strings.TrimSpace(ifName) == "" {
		return true
	}
	mtu, err := strconv.Atoi(s)
	if err != nil || mtu < 576 || mtu > 9000 {
		log.Printf("connect-ip: CONNECT_IP_TUN_MTU=%q invalid or out of range [576,9000]; skipping mtu", s)
		return false
	}
	if _, err := exec.LookPath("ip"); err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_MTU: ip not in PATH: %v", err)
		return false
	}
	out, err := exec.Command("ip", "link", "set", "dev", ifName, "mtu", strconv.Itoa(mtu)).CombinedOutput()
	if err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_MTU: ip link set dev %s mtu %d: %v: %s", ifName, mtu, err, strings.TrimSpace(string(out)))
		return false
	}
	log.Printf("connect-ip: set dev %s mtu %d", ifName, mtu)
	return true
}
