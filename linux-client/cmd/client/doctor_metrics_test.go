package main

import "testing"

func TestCapabilitiesTUNBridgeAdvertised(t *testing.T) {
	const withTun = `{
  "tunnel": {
    "quic": {
      "connect_ip": {
        "http3_datagrams": { "tun_linux_per_session": true }
      }
    }
  }
}`
	const noTun = `{
  "tunnel": {
    "quic": {
      "connect_ip": {
        "http3_datagrams": { "echo": true }
      }
    }
  }
}`
	if !capabilitiesTUNBridgeAdvertised([]byte(withTun)) {
		t.Fatal("expected true for tun_linux_per_session")
	}
	if capabilitiesTUNBridgeAdvertised([]byte(noTun)) {
		t.Fatal("expected false without tun_linux_per_session")
	}
	if capabilitiesTUNBridgeAdvertised([]byte(`{`)) {
		t.Fatal("expected false for invalid JSON")
	}
}
