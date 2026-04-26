package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"afbuyers/masque-server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Set at link time: -X main.version=... -X main.commit=... -X main.date=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type tunnelRequest struct {
	DeviceToken     string `json:"device_token"`
	Fingerprint     string `json:"fingerprint"`
	DestinationIP   string `json:"destination_ip"`
	DestinationPort int    `json:"destination_port"`
	Protocol        string `json:"protocol"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("masque-server %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built:  %s\n", date)
		fmt.Printf("go:     %s\n", runtime.Version())
		return
	}

	cpURL := envOr("CONTROL_PLANE_URL", "http://127.0.0.1:8000")
	listenAddr := envOr("LISTEN_ADDR", ":8443")
	metricsRegistry := prometheus.NewRegistry()
	metrics := newServerMetrics(metricsRegistry)

	cpClient := auth.NewClient(cpURL)
	router := chi.NewRouter()

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		metrics.healthChecksTotal.Inc()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": version,
			"ts":      time.Now().UTC(),
		})
	})
	router.Handle("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))

	router.Post("/connect", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		metrics.connectRequestsTotal.Inc()

		var req tunnelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			metrics.connectFailuresTotal.WithLabelValues("bad_payload").Inc()
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}

		authz, err := cpClient.Authorize(r.Context(), req.DeviceToken, req.Fingerprint)
		if err != nil {
			metrics.connectFailuresTotal.WithLabelValues("unauthorized").Inc()
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		if req.DestinationIP != "" && !auth.IsAllowed(authz.ACL, req.DestinationIP, req.Protocol, req.DestinationPort) {
			metrics.connectFailuresTotal.WithLabelValues("acl_denied").Inc()
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "acl denied"})
			return
		}
		metrics.connectSuccessTotal.Inc()
		metrics.connectLatency.Observe(time.Since(start).Seconds())

		writeJSON(w, http.StatusOK, map[string]any{
			"session":      "minimal-masque-session",
			"user_id":      authz.UserID,
			"device_id":    authz.DeviceID,
			"policy_acl":   authz.ACL,
			"routes":       authz.Routes,
			"dns":          authz.DNS,
			"masque_ready": true,
		})
	})

	log.Printf("masque-server listening on %s, control-plane=%s", listenAddr, cpURL)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func envOr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

type serverMetrics struct {
	connectRequestsTotal prometheus.Counter
	connectSuccessTotal  prometheus.Counter
	connectFailuresTotal *prometheus.CounterVec
	connectLatency       prometheus.Histogram
	healthChecksTotal    prometheus.Counter
}

func newServerMetrics(registry *prometheus.Registry) *serverMetrics {
	m := &serverMetrics{
		connectRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_requests_total",
			Help: "Total connect API requests.",
		}, []string{}).WithLabelValues(),
		connectSuccessTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_connect_success_total",
			Help: "Total successful connect API requests.",
		}),
		connectFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "masque_connect_failures_total",
			Help: "Total failed connect API requests by reason.",
		}, []string{"reason"}),
		connectLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "masque_connect_latency_seconds",
			Help:    "Latency of connect API handler.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.3, 0.5, 1, 2, 5},
		}),
		healthChecksTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "masque_healthz_requests_total",
			Help: "Total health endpoint hits.",
		}),
	}

	registry.MustRegister(
		m.connectRequestsTotal,
		m.connectSuccessTotal,
		m.connectFailuresTotal,
		m.connectLatency,
		m.healthChecksTotal,
	)

	return m
}
