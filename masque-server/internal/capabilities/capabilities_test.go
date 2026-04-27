package capabilities

import (
	"strings"
	"testing"
)

func TestBuild_QUICOff(t *testing.T) {
	doc := Build(Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		QUICListenAddr:      "",
	})
	if doc["version"] != "test" {
		t.Fatalf("version: %v", doc["version"])
	}
	tr := doc["transport"].(map[string]any)
	if _, ok := tr["http3_stub"]; ok {
		t.Fatal("expected no http3_stub when QUIC off")
	}
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	if quic["enabled"] != false {
		t.Fatalf("quic.enabled: %v", quic["enabled"])
	}
	p2b := tun["phase2b"].(map[string]any)
	if p2b["connect_ip_quic_stub"] != false {
		t.Fatalf("phase2b.connect_ip_quic_stub: %v", p2b["connect_ip_quic_stub"])
	}
}

func TestBuild_QUICOn(t *testing.T) {
	doc := Build(Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000/",
		QUICListenAddr:      ":8444",
	})
	tr := doc["transport"].(map[string]any)
	stub, ok := tr["http3_stub"].(map[string]any)
	if !ok {
		t.Fatal("expected http3_stub")
	}
	if stub["listen_udp"] != ":8444" {
		t.Fatalf("listen_udp: %v", stub["listen_udp"])
	}
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	if quic["enabled"] != true {
		t.Fatalf("quic.enabled: %v", quic["enabled"])
	}
	cp := tun["control_plane"].(map[string]any)
	if cp["authorize_url"] != "http://127.0.0.1:8000/api/v1/server/authorize" {
		t.Fatalf("authorize_url: %v", cp["authorize_url"])
	}
	p2a := tun["phase2a"].(map[string]any)
	if p2a["tcp_server_probe"] != true {
		t.Fatalf("phase2a.tcp_server_probe: %v", p2a["tcp_server_probe"])
	}
	p2b := tun["phase2b"].(map[string]any)
	if p2b["connect_ip_quic_stub"] != true {
		t.Fatalf("phase2b.connect_ip_quic_stub: %v", p2b["connect_ip_quic_stub"])
	}
	ci := quic["connect_ip"].(map[string]any)
	dg := ci["http3_datagrams"].(map[string]any)
	if dg["udp_ipv4_relay"] != nil {
		t.Fatalf("expected no udp_ipv4_relay when relay off, got %v", dg["udp_ipv4_relay"])
	}
}

func TestBuild_QUICOnUDPRelay(t *testing.T) {
	doc := Build(Params{
		Version:               "test",
		TCPListenAddr:         ":8443",
		ControlPlaneBaseURL:   "http://127.0.0.1:8000",
		QUICListenAddr:        ":8444",
		ConnectIPUDPRelayIPv4: true,
	})
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	ci := quic["connect_ip"].(map[string]any)
	dg := ci["http3_datagrams"].(map[string]any)
	if dg["udp_ipv4_relay"] != true {
		t.Fatalf("udp_ipv4_relay: %v", dg["udp_ipv4_relay"])
	}
	if dg["ip_forward"] != "ipv4_udp_user_space" {
		t.Fatalf("ip_forward: %v", dg["ip_forward"])
	}
}

func TestBuild_QUICOnICMPRelay(t *testing.T) {
	doc := Build(Params{
		Version:                "test",
		TCPListenAddr:          ":8443",
		ControlPlaneBaseURL:    "http://127.0.0.1:8000",
		QUICListenAddr:         ":8444",
		ConnectIPICMPRelayIPv4: true,
	})
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	ci := quic["connect_ip"].(map[string]any)
	dg := ci["http3_datagrams"].(map[string]any)
	if dg["icmp_ipv4_echo_relay"] != true {
		t.Fatalf("icmp_ipv4_echo_relay: %v", dg["icmp_ipv4_echo_relay"])
	}
	if dg["ip_forward"] != "ipv4_icmp_user_space" {
		t.Fatalf("ip_forward: %v", dg["ip_forward"])
	}
}

func TestBuild_QUICOnUDPAndICMPRelay(t *testing.T) {
	doc := Build(Params{
		Version:                "test",
		TCPListenAddr:          ":8443",
		ControlPlaneBaseURL:    "http://127.0.0.1:8000",
		QUICListenAddr:         ":8444",
		ConnectIPUDPRelayIPv4:  true,
		ConnectIPICMPRelayIPv4: true,
	})
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	ci := quic["connect_ip"].(map[string]any)
	dg := ci["http3_datagrams"].(map[string]any)
	if dg["ip_forward"] != "ipv4_udp_icmp_user_space" {
		t.Fatalf("ip_forward: %v", dg["ip_forward"])
	}
}

func TestBuild_MainListenerTLS(t *testing.T) {
	doc := Build(Params{
		Version:             "t",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		MainListenerTLS:     true,
	})
	tr := doc["transport"].(map[string]any)
	tls, ok := tr["tls"].(map[string]any)
	if !ok || tls["enabled"] != true {
		t.Fatalf("transport.tls: %v", tr["tls"])
	}
}

func TestBuild_QUICOnTunKernelForward(t *testing.T) {
	doc := Build(Params{
		Version:                   "test",
		TCPListenAddr:             ":8443",
		ControlPlaneBaseURL:       "http://127.0.0.1:8000",
		QUICListenAddr:            ":8444",
		ConnectIPTunKernelForward: true,
	})
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	ci := quic["connect_ip"].(map[string]any)
	dg := ci["http3_datagrams"].(map[string]any)
	if dg["tun_linux_per_session"] != true {
		t.Fatalf("tun_linux_per_session: %v", dg["tun_linux_per_session"])
	}
	note := dg["note"].(string)
	if !strings.Contains(note, "CONNECT_IP_TUN_FORWARD") {
		t.Fatalf("http3_datagrams.note should mention CONNECT_IP_TUN_FORWARD: %q", note)
	}
	dev := ci["dev"].(map[string]any)
	if _, ok := dev["tun_forward_env"]; !ok {
		t.Fatal("expected dev.tun_forward_env")
	}
	rfc := ci["rfc9484"].(map[string]any)
	ni := rfc["not_implemented"].([]string)
	var joined string
	for _, s := range ni {
		joined += s + " "
	}
	if !strings.Contains(joined, "in-process SNAT") {
		t.Fatalf("not_implemented: %v", ni)
	}
}

func TestBuild_QUICOnRoutePushCIDR(t *testing.T) {
	doc := Build(Params{
		Version:                      "test",
		TCPListenAddr:                ":8443",
		ControlPlaneBaseURL:          "http://127.0.0.1:8000",
		QUICListenAddr:               ":8444",
		ConnectIPRouteAdvertPushCIDR: "198.18.0.0/15",
	})
	tun := doc["tunnel"].(map[string]any)
	quic := tun["quic"].(map[string]any)
	ci := quic["connect_ip"].(map[string]any)
	rp, ok := ci["route_push"].(map[string]any)
	if !ok {
		t.Fatal("expected route_push")
	}
	if rp["cidr"] != "198.18.0.0/15" {
		t.Fatalf("cidr: %v", rp["cidr"])
	}
}
