//go:build linux

package http3stub

import (
	"log"
	"os/exec"
	"strings"
)

// maybeBringUpConnectIPTun runs `ip link set dev <if> up` when enabled (CONNECT_IP_TUN_LINK_UP).
func maybeBringUpConnectIPTun(ifName string, enabled bool) {
	if !enabled || strings.TrimSpace(ifName) == "" {
		return
	}
	if _, err := exec.LookPath("ip"); err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_LINK_UP: ip not in PATH: %v", err)
		return
	}
	out, err := exec.Command("ip", "link", "set", "dev", ifName, "up").CombinedOutput()
	if err != nil {
		log.Printf("connect-ip: CONNECT_IP_TUN_LINK_UP: ip link set dev %s up: %v: %s", ifName, err, strings.TrimSpace(string(out)))
	}
}
