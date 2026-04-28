package http3stub

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"afbuyers/masque-server/internal/capabilities"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// ListenAndServe starts an HTTP/3 server with CONNECT-IP optional auth disabled (tests / minimal).
func ListenAndServe(p capabilities.Params) error {
	return Listen(ListenConfig{Params: p})
}

// Listen starts an HTTP/3 server on UDP at cfg.Params.QUICListenAddr.
// TLS uses an ephemeral self-signed certificate (development only).
func Listen(cfg ListenConfig) error {
	addr := strings.TrimSpace(cfg.Params.QUICListenAddr)
	if addr == "" {
		return fmt.Errorf("http3stub: QUICListenAddr is required")
	}

	cert, err := ephemeralLeafCert()
	if err != nil {
		return err
	}

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{http3.NextProtoH3},
	}

	srv := http3.Server{
		Addr:            addr,
		Handler:         newHandler(cfg),
		TLSConfig:       tlsConf,
		// Align with VPN-style CONNECT-IP clients: negotiated idle timeout is min(client, server).
		QUICConfig: &quic.Config{
			EnableDatagrams: true,
			MaxIdleTimeout:  30 * time.Minute,
		},
		EnableDatagrams: true, // RFC 9297; required path for CONNECT-IP payloads once relay exists
	}

	log.Printf("masque-server http3 stub listening UDP %s (self-signed TLS); CONNECT-IP stub (:protocol connect-ip)", addr)
	return srv.ListenAndServe()
}

func newHandler(cfg ListenConfig) http.Handler {
	p := cfg.Params
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": p.Version,
			"ts":      time.Now().UTC(),
			"via":     "http3",
		})
	})
	mux.HandleFunc("/v1/masque/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, capabilities.Build(p))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isConnectIPRequest(r) {
			serveConnectIPStub(w, r, cfg)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func ephemeralLeafCert() (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "masque-server-http3-stub",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}
