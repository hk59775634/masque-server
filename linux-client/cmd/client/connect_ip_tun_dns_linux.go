//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// applyTunDNS backs up /etc/resolv.conf, overwrites with nameserver lines, and returns restore.
func applyTunDNS(servers []string) (restore func(), err error) {
	if len(servers) == 0 {
		return func() {}, nil
	}
	warnIfStubResolvConf()
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

func warnIfStubResolvConf() {
	target, err := os.Readlink("/etc/resolv.conf")
	if err != nil {
		return
	}
	if strings.Contains(target, "systemd/resolve/stub-resolv.conf") {
		log.Printf("connect-ip-tun: warn: /etc/resolv.conf -> %q (systemd-resolved stub); overwriting may not affect apps using resolved directly — consider `resolvectl dns` or manage DNS outside this CLI", target)
	}
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
