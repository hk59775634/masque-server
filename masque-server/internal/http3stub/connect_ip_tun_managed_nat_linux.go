//go:build linux

package http3stub

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
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
	case table == "mangle":
		addArgs = append([]string{"-t", "mangle", "-A"}, ruleArgs...)
	default:
		addArgs = append([]string{"-A"}, ruleArgs...)
	}
	return runCombined("iptables", addArgs...)
}

func ensureNftTable(family string, table string) error {
	if err := runCombined("nft", "add", "table", family, table); err != nil && !strings.Contains(err.Error(), "File exists") {
		return err
	}
	return nil
}

func ensureNftBaseChain(family string, table string, chain string, chainType string, hook string, priority int) error {
	if err := ensureNftTable(family, table); err != nil {
		return err
	}
	args := []string{
		"add", "chain", family, table, chain,
		"{", "type", chainType, "hook", hook, "priority", strconv.Itoa(priority), ";", "policy", "accept", ";", "}",
	}
	if err := runCombined("nft", args...); err != nil && !strings.Contains(err.Error(), "File exists") {
		return err
	}
	return runCombined("nft", "list", "chain", family, table, chain)
}

func ensureNftRule(family string, table string, chain string, expr string) error {
	if err := ensureNftTable(family, table); err != nil {
		return err
	}
	if err := runCombined("nft", "add", "chain", family, table, chain); err != nil && !strings.Contains(err.Error(), "File exists") {
		return err
	}
	if err := runCombined("nft", "list", "chain", family, table, chain); err != nil {
		return err
	}
	// Duplicate-safe add: rely on -a list and textual check.
	out, _ := exec.Command("nft", "-a", "list", "chain", family, table, chain).CombinedOutput()
	if strings.Contains(string(out), expr) {
		return nil
	}
	return runCombined("nft", "add", "rule", family, table, chain, expr)
}

func connectIPTunClampMSS() int {
	if s := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_TCP_MSS")); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n >= 536 && n <= 1460 {
			return n
		}
		log.Printf("connect-ip: CONNECT_IP_TUN_TCP_MSS=%q invalid; using MTU-derived MSS", s)
	}
	if s := strings.TrimSpace(os.Getenv("CONNECT_IP_TUN_MTU")); s != "" {
		mtu, err := strconv.Atoi(s)
		if err == nil && mtu >= 576 {
			mss := mtu - 40
			if mss >= 536 {
				return mss
			}
		}
	}
	return 1240 // 1280 MTU − 40 B IP+TCP headers (common VPN/tunnel padding)
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
	markBackend := func(backend, result string) {
		if cfg.ConnectIPTunManagedNATBackendResults != nil {
			cfg.ConnectIPTunManagedNATBackendResults.WithLabelValues(backend, result).Inc()
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
	mss := connectIPTunClampMSS()
	backend := strings.TrimSpace(strings.ToLower(cfg.ConnectIPTunManagedNATBackend))
	if backend == "" {
		backend = "nftables"
	}
	applyIPTables := func() error {
		if _, err := exec.LookPath("iptables"); err != nil {
			return fmt.Errorf("iptables not in PATH: %w", err)
		}
		// Allow forward tun -> egress and return traffic egress -> tun.
		if err := ensureIptablesRule("", "FORWARD", "-i", ifName, "-o", egress, "-j", "ACCEPT"); err != nil {
			return err
		}
		if err := ensureIptablesRule("", "FORWARD", "-i", egress, "-o", ifName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
			return err
		}
		if err := ensureIptablesRule("nat", "POSTROUTING", "-o", egress, "-j", "MASQUERADE"); err != nil {
			return err
		}
		// MSS clamp on traffic touching the TUN (local termination + forwarded): avoids SYN-ACK advertising 1460 on a 1280 path.
		if err := ensureIptablesRule("mangle", "PREROUTING", "-i", ifName, "-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", strconv.Itoa(mss)); err != nil {
			return err
		}
		if err := ensureIptablesRule("mangle", "POSTROUTING", "-o", ifName, "-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", strconv.Itoa(mss)); err != nil {
			return err
		}
		return nil
	}
	applyNFTables := func() error {
		if _, err := exec.LookPath("nft"); err != nil {
			return fmt.Errorf("nft not in PATH: %w", err)
		}
		// Use dedicated table/chains to avoid clobbering host rules.
		if err := ensureNftBaseChain("ip", "masque_connect_ip", "forward", "filter", "forward", 0); err != nil {
			return err
		}
		if err := ensureNftBaseChain("ip", "masque_connect_ip", "postrouting", "nat", "postrouting", 100); err != nil {
			return err
		}
		if err := ensureNftBaseChain("ip", "masque_connect_ip", "prerouting", "filter", "prerouting", -150); err != nil {
			return err
		}
		if err := ensureNftBaseChain("ip", "masque_connect_ip", "postrouting_mangle", "filter", "postrouting", -150); err != nil {
			return err
		}
		if err := ensureNftRule("ip", "masque_connect_ip", "forward", fmt.Sprintf("iifname %q oifname %q accept", ifName, egress)); err != nil {
			return err
		}
		if err := ensureNftRule("ip", "masque_connect_ip", "forward", fmt.Sprintf("iifname %q oifname %q ct state established,related accept", egress, ifName)); err != nil {
			return err
		}
		if err := ensureNftRule("ip", "masque_connect_ip", "postrouting", fmt.Sprintf("oifname %q masquerade", egress)); err != nil {
			return err
		}
		if err := ensureNftRule("ip", "masque_connect_ip", "prerouting", fmt.Sprintf("iifname %q tcp flags syn tcp option maxseg size set %d", ifName, mss)); err != nil {
			return err
		}
		if err := ensureNftRule("ip", "masque_connect_ip", "postrouting_mangle", fmt.Sprintf("oifname %q tcp flags syn tcp option maxseg size set %d", ifName, mss)); err != nil {
			return err
		}
		return nil
	}

	tryBackend := func(b string) error {
		switch b {
		case "iptables":
			return applyIPTables()
		case "nftables":
			return applyNFTables()
		default:
			return fmt.Errorf("unsupported CONNECT_IP_TUN_NAT_BACKEND=%q (want nftables or iptables)", b)
		}
	}
	if err := tryBackend(backend); err != nil {
		markBackend(backend, "error")
		if backend == "nftables" && cfg.ConnectIPTunManagedNATAllowIPTablesFallback {
			log.Printf("connect-ip: managed NAT nftables apply failed on %s; falling back to iptables: %v", ifName, err)
			markBackend("nftables", "fallback")
			if err2 := tryBackend("iptables"); err2 != nil {
				markBackend("iptables", "error")
				return fail(fmt.Errorf("nftables failed (%v); iptables fallback failed (%v)", err, err2))
			}
			markBackend("iptables", "ok")
			mark("ok")
			log.Printf("connect-ip: managed NAT applied on %s via iptables fallback (egress=%s addr=%q tcp_mss=%d)", ifName, egress, strings.TrimSpace(cfg.ConnectIPTunAddressCIDR), mss)
			return true
		}
		return fail(err)
	}
	markBackend(backend, "ok")
	mark("ok")
	log.Printf("connect-ip: managed NAT applied on %s via %s (egress=%s addr=%q tcp_mss=%d)", ifName, backend, egress, strings.TrimSpace(cfg.ConnectIPTunAddressCIDR), mss)
	return true
}
