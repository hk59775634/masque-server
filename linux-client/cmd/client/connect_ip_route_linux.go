//go:build linux

package main

import (
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"strings"
)

// installSplitDefaultAndBypass adds IPv4 split-default via TUN and optional /32 bypass for the QUIC server
// (and masque HTTPS URL host when it differs), so traffic to the server does not enter the tunnel.
func installSplitDefaultAndBypass(ifName, udpHostPort, masqueBaseURL string, bypassHost bool) (cleanup func(), err error) {
	udpHost, _, err := net.SplitHostPort(udpHostPort)
	if err != nil {
		return nil, fmt.Errorf("udp target: %w", err)
	}
	udpHost = stripIPv6Brackets(udpHost)

	var bypassCIDRs []string
	if bypassHost {
		gw, dev, gerr := defaultIPv4Gateway()
		if gerr != nil {
			return nil, fmt.Errorf("bypass host: default route: %w", gerr)
		}
		seen := map[string]struct{}{}
		addBypass := func(host string) error {
			if host == "" {
				return nil
			}
			ip, rerr := resolveIPv4(host)
			if rerr != nil {
				return fmt.Errorf("resolve %q: %w", host, rerr)
			}
			cidr := ip.String() + "/32"
			if _, ok := seen[cidr]; ok {
				return nil
			}
			seen[cidr] = struct{}{}
			if err := runIP("route", "replace", cidr, "via", gw.String(), "dev", dev); err != nil {
				return fmt.Errorf("ip route replace %s via %s dev %s: %w", cidr, gw, dev, err)
			}
			bypassCIDRs = append(bypassCIDRs, cidr)
			return nil
		}
		if err := addBypass(udpHost); err != nil {
			return nil, err
		}
		if u, uerr := url.Parse(masqueBaseURL); uerr == nil && u != nil {
			h := strings.TrimSpace(u.Hostname())
			if h != "" && h != udpHost {
				if err := addBypass(h); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := runIP("route", "replace", "0.0.0.0/1", "dev", ifName); err != nil {
		undoBypass(bypassCIDRs)
		return nil, fmt.Errorf("ip route replace 0.0.0.0/1 dev %s: %w", ifName, err)
	}
	if err := runIP("route", "replace", "128.0.0.0/1", "dev", ifName); err != nil {
		_ = runIP("route", "del", "0.0.0.0/1", "dev", ifName)
		undoBypass(bypassCIDRs)
		return nil, fmt.Errorf("ip route replace 128.0.0.0/1 dev %s: %w", ifName, err)
	}

	return func() {
		_ = runIP("route", "del", "0.0.0.0/1", "dev", ifName)
		_ = runIP("route", "del", "128.0.0.0/1", "dev", ifName)
		undoBypass(bypassCIDRs)
	}, nil
}

func undoBypass(cidrs []string) {
	for i := len(cidrs) - 1; i >= 0; i-- {
		_ = runIP("route", "del", cidrs[i])
	}
}

func resolveIPv4(host string) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
		return nil, fmt.Errorf("need IPv4 for bypass, got %v", ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4, nil
		}
	}
	return nil, fmt.Errorf("no A record for %q", host)
}

func defaultIPv4Gateway() (gw net.IP, dev string, err error) {
	out, err := exec.Command("ip", "-4", "route", "show", "default").CombinedOutput()
	if err != nil {
		return nil, "", fmt.Errorf("ip -4 route show default: %w: %s", err, strings.TrimSpace(string(out)))
	}
	gws, devs, perr := parseIPv4DefaultRoutes(string(out))
	if perr != nil {
		return nil, "", perr
	}
	return gws[0], devs[0], nil
}

// parseIPv4DefaultRoutes parses `ip -4 route show default` output; returns one entry per "default" line with IPv4 gateway.
func parseIPv4DefaultRoutes(s string) (gateways []net.IP, devs []string, err error) {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		gw, dev, ok := parseOneIPv4DefaultLine(line)
		if !ok {
			continue
		}
		ip := net.ParseIP(gw)
		if ip == nil || ip.To4() == nil {
			continue
		}
		gateways = append(gateways, ip.To4())
		devs = append(devs, dev)
	}
	if len(gateways) == 0 {
		return nil, nil, fmt.Errorf("no IPv4 default route with `via` in ip output: %q", strings.TrimSpace(s))
	}
	return gateways, devs, nil
}

func parseOneIPv4DefaultLine(line string) (gw, dev string, ok bool) {
	if !strings.HasPrefix(line, "default") {
		return "", "", false
	}
	fields := strings.Fields(line)
	// default via 192.168.1.1 dev eth0 ...
	viaAt := -1
	devAt := -1
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			viaAt = i + 1
		}
		if f == "dev" && i+1 < len(fields) {
			devAt = i + 1
		}
	}
	if viaAt < 0 || devAt < 0 {
		return "", "", false
	}
	return fields[viaAt], fields[devAt], true
}
