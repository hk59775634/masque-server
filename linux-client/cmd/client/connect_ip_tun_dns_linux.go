//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// applyTunDNS backs up /etc/resolv.conf, overwrites with nameserver lines, and returns restore.
// applyTunDNSViaResolvectl sets per-link DNS on the IPv4 default-route interface (systemd-resolved).
func applyTunDNSViaResolvectl(servers []string) (restore func(), err error) {
	if len(servers) == 0 {
		return func() {}, nil
	}
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return nil, fmt.Errorf("resolvectl not in PATH: %w", err)
	}
	_, dev, err := defaultIPv4Gateway()
	if err != nil {
		return nil, fmt.Errorf("default IPv4 interface: %w", err)
	}
	args := append([]string{"dns", dev}, servers...)
	out, err := exec.Command("resolvectl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("resolvectl %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return func() {
		_ = exec.Command("resolvectl", "revert", dev).Run()
	}, nil
}

func applyTunDNS(servers []string) (restore func(), err error) {
	if len(servers) == 0 {
		return func() {}, nil
	}
	if isStubResolvConf() {
		log.Printf("connect-ip-tun: warn: /etc/resolv.conf points at systemd-resolved stub; overwriting may not affect all apps — use -dns-resolvectl on systems with systemd-resolved")
	}
	backup, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		backup = nil
	}
	var b strings.Builder
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		b.WriteString("nameserver ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return func() {}, nil
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write /etc/resolv.conf: %w", err)
	}
	return func() {
		if backup == nil {
			return
		}
		if err := os.WriteFile("/etc/resolv.conf", backup, 0o644); err != nil {
			log.Printf("connect-ip-tun: warn: restore /etc/resolv.conf: %v", err)
		}
	}, nil
}

func isStubResolvConf() bool {
	target, err := os.Readlink("/etc/resolv.conf")
	if err != nil {
		return false
	}
	return strings.Contains(target, "systemd/resolve/stub-resolv.conf")
}

func parseCommaList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
