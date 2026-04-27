//go:build linux

package http3stub

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func runCombined(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureIptablesRule(table string, ruleArgs ...string) error {
	iptablesArgs := append([]string{}, ruleArgs...)
	if table != "" {
		iptablesArgs = append([]string{"-t", table}, iptablesArgs...)
	}
	checkArgs := append([]string{"-C"}, iptablesArgs...)
	if err := runCombined("iptables", checkArgs...); err == nil {
		return nil
	}
	var addArgs []string
	switch {
	case table == "nat":
		addArgs = append([]string{"-t", "nat", "-A"}, ruleArgs...)
	default:
		addArgs = append([]string{"-A"}, ruleArgs...)
	}
	return runCombined("iptables", addArgs...)
}

func maybeConfigureConnectIPTunManagedNAT(ifName string, cfg ListenConfig) bool {
	if !cfg.ConnectIPTunManagedNAT {
		return true
	}
	mark := func(result string) {
		if cfg.ConnectIPTunManagedNATApplyResults != nil {
			cfg.ConnectIPTunManagedNATApplyResults.WithLabelValues(result).Inc()
		}
	}
	fail := func(err error) bool {
		log.Printf("connect-ip: managed NAT apply failed on %s: %v", ifName, err)
		mark("error")
		return false
	}
	if strings.TrimSpace(ifName) == "" {
		return fail(fmt.Errorf("empty tun interface name"))
	}
	egress := strings.TrimSpace(cfg.ConnectIPTunEgressInterface)
	if egress == "" {
		return fail(fmt.Errorf("CONNECT_IP_TUN_EGRESS_IFACE is required when CONNECT_IP_TUN_MANAGED_NAT=1"))
	}
	if _, err := exec.LookPath("sysctl"); err != nil {
		return fail(fmt.Errorf("sysctl not in PATH: %w", err))
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		return fail(fmt.Errorf("iptables not in PATH: %w", err))
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return fail(fmt.Errorf("ip not in PATH: %w", err))
	}
	if err := runCombined("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fail(err)
	}
	if s := strings.TrimSpace(cfg.ConnectIPTunAddressCIDR); s != "" {
		if err := runCombined("ip", "addr", "replace", s, "dev", ifName); err != nil {
			return fail(err)
		}
	}
	// Allow forward tun -> egress and return traffic egress -> tun.
	if err := ensureIptablesRule("", "FORWARD", "-i", ifName, "-o", egress, "-j", "ACCEPT"); err != nil {
		return fail(err)
	}
	if err := ensureIptablesRule("", "FORWARD", "-i", egress, "-o", ifName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fail(err)
	}
	if err := ensureIptablesRule("nat", "POSTROUTING", "-o", egress, "-j", "MASQUERADE"); err != nil {
		return fail(err)
	}
	mark("ok")
	log.Printf("connect-ip: managed NAT applied on %s (egress=%s addr=%q)", ifName, egress, strings.TrimSpace(cfg.ConnectIPTunAddressCIDR))
	return true
}
