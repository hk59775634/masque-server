//go:build linux

package main

import (
	"net"
	"testing"
)

func TestParseOneIPv4DefaultLine(t *testing.T) {
	gw, dev, ok := parseOneIPv4DefaultLine("default via 192.168.1.1 dev eth0 proto dhcp metric 100")
	if !ok || gw != "192.168.1.1" || dev != "eth0" {
		t.Fatalf("got gw=%q dev=%q ok=%v", gw, dev, ok)
	}
	_, _, ok = parseOneIPv4DefaultLine("192.168.0.0/24 dev eth0 proto kernel scope link src 192.168.0.5")
	if ok {
		t.Fatal("expected non-default line to fail")
	}
}

func TestParseIPv4DefaultRoutes(t *testing.T) {
	s := "default via 10.0.0.1 dev wlan0 proto dhcp\n"
	gws, devs, err := parseIPv4DefaultRoutes(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(gws) != 1 || !gws[0].Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("gateways=%v", gws)
	}
	if len(devs) != 1 || devs[0] != "wlan0" {
		t.Fatalf("devs=%v", devs)
	}
}
