package http3stub

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"afbuyers/masque-server/internal/auth"
	"afbuyers/masque-server/internal/capabilities"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type fakeAuthorizer struct{}

func (fakeAuthorizer) Authorize(ctx context.Context, token, fp string) (*auth.AuthorizeResponse, error) {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(fp) == "" {
		return nil, errors.New("missing")
	}
	return &auth.AuthorizeResponse{Allowed: true, DeviceID: 1, UserID: 2, ACL: map[string]any{}}, nil
}

func TestConnectIPStubExtendedConnect(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	cert, err := ephemeralLeafCert()
	if err != nil {
		t.Fatal(err)
	}

	p := capabilities.Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		QUICListenAddr:      pc.LocalAddr().String(),
	}

	srv := &http3.Server{
		Handler:         newHandler(ListenConfig{Params: p}),
		EnableDatagrams: true,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{http3.NextProtoH3},
		},
	}
	go func() {
		if err := srv.Serve(pc); err != nil && err != http.ErrServerClosed {
			t.Logf("http3 Serve: %v", err)
		}
	}()
	defer srv.Close()

	time.Sleep(50 * time.Millisecond)

	port := pc.LocalAddr().(*net.UDPAddr).Port
	tlsClient := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{http3.NextProtoH3},
		ServerName:         "127.0.0.1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	qconf := &quic.Config{EnableDatagrams: true}
	conn, err := quic.DialAddrEarly(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsClient, qconf)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")

	tr := &http3.Transport{EnableDatagrams: true, QUICConfig: qconf}
	cc := tr.NewClientConn(conn)
	select {
	case <-cc.ReceivedSettings():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP/3 SETTINGS")
	}
	if s := cc.Settings(); s == nil || !s.EnableDatagrams {
		t.Fatal("expected server SETTINGS to enable HTTP/3 datagrams")
	}

	reqURL := fmt.Sprintf("https://127.0.0.1:%d/.well-known/masque/connect-ip", port)
	req, err := http.NewRequest(http.MethodConnect, reqURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Proto = connectIPProtocol

	resp, err := cc.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	if resp.Header.Get("Capsule-Protocol") == "" {
		t.Fatal("expected Capsule-Protocol response header")
	}
	_ = resp.Body.Close()
}

func TestConnectIPStubUnauthorizedWithAuthorizer(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	cert, err := ephemeralLeafCert()
	if err != nil {
		t.Fatal(err)
	}

	p := capabilities.Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		QUICListenAddr:      pc.LocalAddr().String(),
	}

	reg := prometheus.NewRegistry()
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "masque_connect_ip_requests_total", Help: "test"}, []string{"result"})
	reg.MustRegister(vec)

	cfg := ListenConfig{
		Params:           p,
		Authorizer:       fakeAuthorizer{},
		ConnectIPResults: vec,
	}

	srv := &http3.Server{
		Handler:         newHandler(cfg),
		EnableDatagrams: true,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{http3.NextProtoH3},
		},
	}
	go func() { _ = srv.Serve(pc) }()
	defer srv.Close()
	time.Sleep(50 * time.Millisecond)

	port := pc.LocalAddr().(*net.UDPAddr).Port
	tlsClient := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}, ServerName: "127.0.0.1"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	qconf := &quic.Config{EnableDatagrams: true}
	conn, err := quic.DialAddrEarly(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsClient, qconf)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")

	tr := &http3.Transport{EnableDatagrams: true, QUICConfig: qconf}
	cc := tr.NewClientConn(conn)
	select {
	case <-cc.ReceivedSettings():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP/3 SETTINGS")
	}

	reqURL := fmt.Sprintf("https://127.0.0.1:%d/.well-known/masque/connect-ip", port)
	req, err := http.NewRequest(http.MethodConnect, reqURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Proto = connectIPProtocol

	resp, err := cc.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", resp.StatusCode)
	}
	if n := testutil.ToFloat64(vec.WithLabelValues("unauthorized")); n != 1 {
		t.Fatalf("unauthorized counter=%v want 1", n)
	}
}

func TestConnectIPStubWithBearerAndFingerprint(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	cert, err := ephemeralLeafCert()
	if err != nil {
		t.Fatal(err)
	}

	p := capabilities.Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		QUICListenAddr:      pc.LocalAddr().String(),
	}

	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "masque_connect_ip_requests_total", Help: "test"}, []string{"result"})

	cfg := ListenConfig{
		Params:           p,
		Authorizer:       fakeAuthorizer{},
		ConnectIPResults: vec,
	}

	srv := &http3.Server{
		Handler:         newHandler(cfg),
		EnableDatagrams: true,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{http3.NextProtoH3},
		},
	}
	go func() { _ = srv.Serve(pc) }()
	defer srv.Close()
	time.Sleep(50 * time.Millisecond)

	port := pc.LocalAddr().(*net.UDPAddr).Port
	tlsClient := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}, ServerName: "127.0.0.1"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	qconf := &quic.Config{EnableDatagrams: true}
	conn, err := quic.DialAddrEarly(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsClient, qconf)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")

	tr := &http3.Transport{EnableDatagrams: true, QUICConfig: qconf}
	cc := tr.NewClientConn(conn)
	select {
	case <-cc.ReceivedSettings():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP/3 SETTINGS")
	}

	reqURL := fmt.Sprintf("https://127.0.0.1:%d/.well-known/masque/connect-ip", port)
	req, err := http.NewRequest(http.MethodConnect, reqURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Proto = connectIPProtocol
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Device-Fingerprint", "fp-test")

	resp, err := cc.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	deadline := time.Now().Add(3 * time.Second)
	var okN float64
	for time.Now().Before(deadline) {
		okN = testutil.ToFloat64(vec.WithLabelValues("ok"))
		if okN >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if okN != 1 {
		t.Fatalf("ok counter=%v want 1", okN)
	}
}

func TestConnectIPStubDatagramEcho(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	cert, err := ephemeralLeafCert()
	if err != nil {
		t.Fatal(err)
	}

	p := capabilities.Params{
		Version:             "test",
		TCPListenAddr:       ":8443",
		ControlPlaneBaseURL: "http://127.0.0.1:8000",
		QUICListenAddr:      pc.LocalAddr().String(),
	}

	srv := &http3.Server{
		Handler:         newHandler(ListenConfig{Params: p}),
		EnableDatagrams: true,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*cert},
			MinVersion:   tls.VersionTLS13,
			NextProtos:   []string{http3.NextProtoH3},
		},
	}
	go func() {
		if err := srv.Serve(pc); err != nil && err != http.ErrServerClosed {
			t.Logf("http3 Serve: %v", err)
		}
	}()
	defer srv.Close()

	time.Sleep(50 * time.Millisecond)

	port := pc.LocalAddr().(*net.UDPAddr).Port
	tlsClient := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{http3.NextProtoH3},
		ServerName:         "127.0.0.1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	qconf := &quic.Config{EnableDatagrams: true}
	conn, err := quic.DialAddrEarly(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsClient, qconf)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "")

	tr := &http3.Transport{EnableDatagrams: true, QUICConfig: qconf}
	defer tr.Close()
	cc := tr.NewClientConn(conn)
	select {
	case <-cc.ReceivedSettings():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP/3 SETTINGS")
	}
	if s := cc.Settings(); s == nil || !s.EnableDatagrams {
		t.Fatal("expected server SETTINGS to enable HTTP/3 datagrams")
	}

	rs, err := cc.OpenRequestStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()

	reqURL := fmt.Sprintf("https://127.0.0.1:%d/.well-known/masque/connect-ip", port)
	req, err := http.NewRequest(http.MethodConnect, reqURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Proto = connectIPProtocol
	if err := rs.SendRequestHeader(req); err != nil {
		t.Fatal(err)
	}

	resp, err := rs.ReadResponse()
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}

	payload := append([]byte{0x00}, []byte("stub-datagram-echo")...) // RFC 9484 Context ID 0 + opaque
	if err := rs.SendDatagram(payload); err != nil {
		t.Fatal(err)
	}
	back, err := rs.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, payload) {
		t.Fatalf("echo mismatch: got %q", back)
	}

	// Second datagram: RFC 9484 Context ID 0 + minimal IPv4/UDP (same shape as linux-client doctor -connect-ip-rfc9484-udp).
	var ip []byte
	ip = append(ip, 0x45, 0x00)
	ip = binary.BigEndian.AppendUint16(ip, 28)
	ip = append(ip, 0x00, 0x00, 0x00, 0x00, 64, 17, 0x00, 0x00)
	ip = append(ip, 192, 0, 2, 10, 192, 0, 2, 1)
	ip = append(ip, 0xde, 0xed, 0x00, 0x35)
	ip = binary.BigEndian.AppendUint16(ip, 8)
	ip = append(ip, 0x00, 0x00)
	pkt := encodeRFC9484ContextZeroIPPacket(ip)
	if err := rs.SendDatagram(pkt); err != nil {
		t.Fatal(err)
	}
	back2, err := rs.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back2, pkt) {
		t.Fatalf("rfc9484 ipv4/udp echo mismatch len sent=%d got=%d", len(pkt), len(back2))
	}
}
