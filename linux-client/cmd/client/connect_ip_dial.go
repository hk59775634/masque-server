package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// connectIPSession holds an established CONNECT-IP HTTP/3 request stream (RFC 9484 over QUIC).
type connectIPSession struct {
	UDPAddr string
	Conn    *quic.Conn
	Tr      *http3.Transport
	CC      *http3.ClientConn
	RS      *http3.RequestStream
	Resp    *http.Response
}

// Close releases QUIC and HTTP/3 resources (reverse order of setup).
func (s *connectIPSession) Close() {
	if s == nil {
		return
	}
	if s.RS != nil {
		_ = s.RS.Close()
	}
	if s.Tr != nil {
		_ = s.Tr.Close()
	}
	if s.Conn != nil {
		s.Conn.CloseWithError(0, "")
	}
}

// defaultConnectIPQUICConfig matches long-lived VPN-style CONNECT-IP use: quiet TUN must not
// drop the QUIC session every 30s (quic-go default idle timeout).
func defaultConnectIPQUICConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		MaxIdleTimeout:  30 * time.Minute,
		KeepAlivePeriod: 15 * time.Second,
	}
}

// ipv4QUICDialAddr returns a literal IPv4:port for QUIC and the TLS ServerName (hostname for SNI
// when dialing by IP after DNS). IPv6 literals are rejected so broken AAAA paths cannot stall dial.
func ipv4QUICDialAddr(ctx context.Context, udpHostPort string) (dialAddr, tlsServerName string, err error) {
	host, port, err := net.SplitHostPort(udpHostPort)
	if err != nil {
		return "", "", fmt.Errorf("UDP address %q: %w", udpHostPort, err)
	}
	host = stripIPv6Brackets(host)
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return "", "", fmt.Errorf("CONNECT-IP QUIC requires IPv4 or a hostname with an A record; got IPv6 literal %q", host)
		}
		return udpHostPort, host, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return "", "", fmt.Errorf("IPv4 DNS lookup %q: %w", host, err)
	}
	if len(ips) == 0 {
		return "", "", fmt.Errorf("no IPv4 address for %q", host)
	}
	v4 := ips[0].To4()
	if v4 == nil {
		return "", "", fmt.Errorf("resolver returned non-IPv4 for %q", host)
	}
	return net.JoinHostPort(v4.String(), port), host, nil
}

// dialConnectIP performs QUIC + HTTP/3 extended CONNECT :protocol connect-ip until HTTP 200.
// quicConfig may be nil to use defaults (30m idle, 15s keepalive). Non-nil configs are cloned
// and EnableDatagrams is forced on.
// The caller must read or drain resp.Body (e.g. capsules); see connectIPTunDrainResponseBody.
func dialConnectIP(ctx context.Context, udpHostPort, deviceToken, fingerprint string, quicConfig *quic.Config) (*connectIPSession, error) {
	dialAddr, tlsServerName, err := ipv4QUICDialAddr(ctx, udpHostPort)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{http3.NextProtoH3},
		ServerName:         tlsServerName,
	}

	// quicConfig: per-dial override; if nil, use long idle + keepalive. The quic-go
	// default MaxIdleTimeout is 30s with no keepalive, which drops the CONNECT-IP
	// response body (capsule) and the whole session when the TUN is quiet.
	quicConf := quicConfig
	if quicConf == nil {
		quicConf = defaultConnectIPQUICConfig()
	} else {
		q := *quicConf
		q.EnableDatagrams = true
		quicConf = &q
	}
	conn, err := quic.DialAddrEarly(ctx, dialAddr, tlsCfg, quicConf)
	if err != nil {
		return nil, fmt.Errorf("QUIC dial %s: %w", dialAddr, err)
	}

	tr := &http3.Transport{
		EnableDatagrams: true,
		QUICConfig:      quicConf,
	}
	cc := tr.NewClientConn(conn)

	select {
	case <-cc.ReceivedSettings():
	case <-ctx.Done():
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("timeout waiting for HTTP/3 SETTINGS from %s", udpHostPort)
	}
	if s := cc.Settings(); s == nil || !s.EnableDatagrams {
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("HTTP/3 SETTINGS from %s: datagrams (RFC 9297) not enabled", udpHostPort)
	}

	rs, err := cc.OpenRequestStream(ctx)
	if err != nil {
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("OpenRequestStream: %w", err)
	}

	u := &url.URL{
		Scheme: "https",
		Host:   udpHostPort,
		Path:   "/.well-known/masque/connect-ip",
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, u.String(), nil)
	if err != nil {
		_ = rs.Close()
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, err
	}
	req.Proto = masqueConnectIPProto
	if strings.TrimSpace(deviceToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(deviceToken))
	}
	if strings.TrimSpace(fingerprint) != "" {
		req.Header.Set("Device-Fingerprint", strings.TrimSpace(fingerprint))
	}

	if err := rs.SendRequestHeader(req); err != nil {
		_ = rs.Close()
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("CONNECT-IP SendRequestHeader: %w", err)
	}

	resp, err := rs.ReadResponse()
	if err != nil {
		_ = rs.Close()
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("CONNECT-IP ReadResponse: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		_ = rs.Close()
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("CONNECT-IP -> HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(slurp)))
	}
	if resp.Header.Get("Capsule-Protocol") == "" {
		_ = resp.Body.Close()
		_ = rs.Close()
		_ = tr.Close()
		conn.CloseWithError(0, "")
		return nil, fmt.Errorf("CONNECT-IP -> 200 but missing Capsule-Protocol header")
	}

	return &connectIPSession{
		UDPAddr: udpHostPort,
		Conn:    conn,
		Tr:      tr,
		CC:      cc,
		RS:      rs,
		Resp:    resp,
	}, nil
}
