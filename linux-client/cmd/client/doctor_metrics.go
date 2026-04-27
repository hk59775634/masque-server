package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// capabilitiesTUNBridgeAdvertised returns true when GET /v1/masque/capabilities reports Linux per-session TUN
// (tunnel.quic.connect_ip.http3_datagrams.tun_linux_per_session).
func capabilitiesTUNBridgeAdvertised(capJSON []byte) bool {
	var cap map[string]any
	if err := json.Unmarshal(capJSON, &cap); err != nil {
		return false
	}
	tun, _ := cap["tunnel"].(map[string]any)
	if tun == nil {
		return false
	}
	quic, _ := tun["quic"].(map[string]any)
	if quic == nil {
		return false
	}
	ci, _ := quic["connect_ip"].(map[string]any)
	if ci == nil {
		return false
	}
	dg, _ := ci["http3_datagrams"].(map[string]any)
	if dg == nil {
		return false
	}
	v, ok := dg["tun_linux_per_session"].(bool)
	return ok && v
}

const (
	metricTUNBridgeActive       = "masque_connect_ip_tun_bridge_active"
	metricTUNOpenEchoFallback = "masque_connect_ip_tun_open_echo_fallback_total"
)

// doctorProbeMasqueCONNECTIPTUNMetrics GETs /metrics and checks that TUN-related series are registered.
func doctorProbeMasqueCONNECTIPTUNMetrics(ctx context.Context, client *http.Client, masqueBaseURL string) (ok bool, detail string) {
	mURL := joinURL(masqueBaseURL, "/metrics")
	raw, code, err := httpGetBody(ctx, client, mURL)
	if err != nil {
		return false, fmt.Sprintf("GET %s: %v", mURL, err)
	}
	if code != http.StatusOK {
		return false, fmt.Sprintf("GET %s returned %d", mURL, code)
	}
	body := string(raw)
	if !strings.Contains(body, metricTUNBridgeActive) {
		return false, fmt.Sprintf("GET %s -> 200 but body lacks %q (older masque-server?)", mURL, metricTUNBridgeActive)
	}
	if !strings.Contains(body, metricTUNOpenEchoFallback) {
		return false, fmt.Sprintf("GET %s -> 200 but body lacks %q (older masque-server?)", mURL, metricTUNOpenEchoFallback)
	}
	return true, fmt.Sprintf("GET %s -> 200; found %s and %s", mURL, metricTUNBridgeActive, metricTUNOpenEchoFallback)
}
