//go:build linux

package http3stub

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/quic-go/quic-go/http3"
)

type sharedTunSession struct {
	id   string
	send func([]byte) error
}

type sharedTunManager struct {
	tun    *os.File
	ifName string
	cfg    ListenConfig

	mu          sync.RWMutex
	sessions    map[string]*sharedTunSession
	ipToSession map[string]sharedTunBinding // client source IP -> session id
}

type sharedTunBinding struct {
	sessionID string
	lastSeen  time.Time
}

var (
	sharedTunMu sync.Mutex
	sharedTun   *sharedTunManager
)

func getOrCreateSharedTunManager(cfg ListenConfig) (*sharedTunManager, error) {
	sharedTunMu.Lock()
	defer sharedTunMu.Unlock()
	if sharedTun != nil {
		return sharedTun, nil
	}
	tun, ifName, cleanup, err := openConnectIPTunForward(cfg.ConnectIPTunName)
	if err != nil {
		return nil, err
	}
	_ = cleanup // shared manager keeps TUN for process lifetime.
	mgr := &sharedTunManager{
		tun:         tun,
		ifName:      ifName,
		cfg:         cfg,
		sessions:    map[string]*sharedTunSession{},
		ipToSession: map[string]sharedTunBinding{},
	}
	if !maybeBringUpConnectIPTun(ifName, cfg.ConnectIPTunLinkUp) && cfg.ConnectIPTunLinkUpFailures != nil {
		cfg.ConnectIPTunLinkUpFailures.Inc()
	}
	if !maybeConfigureConnectIPTunManagedNAT(ifName, cfg) {
		_ = tun.Close()
		return nil, fmt.Errorf("managed NAT apply failed on shared TUN %s", ifName)
	}
	go mgr.readLoop()
	go mgr.gcLoop()
	sharedTun = mgr
	log.Printf("connect-ip: shared TUN manager started if=%s", ifName)
	return mgr, nil
}

func (m *sharedTunManager) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, err := m.tun.Read(buf)
		if err != nil {
			log.Printf("connect-ip shared tun: read %s: %v", m.ifName, err)
			return
		}
		if n <= 0 {
			continue
		}
		pkt := append([]byte(nil), buf[:n]...)
		_, dst, ok := parseConnectIPDatagramEndpoints(pkt)
		if !ok || dst == nil {
			continue
		}
		sessionID := ""
		m.mu.RLock()
		sessionID = m.ipToSession[dst.String()].sessionID
		sess := m.sessions[sessionID]
		m.mu.RUnlock()
		if sess == nil {
			continue
		}
		frame := encodeRFC9484ContextZeroIPPacket(pkt)
		if len(frame) > maxConnectIPDatagramBytes {
			if m.cfg.ConnectIPDatagramDrops != nil {
				m.cfg.ConnectIPDatagramDrops.Inc()
			}
			continue
		}
		if err := sess.send(frame); err != nil {
			log.Printf("connect-ip shared tun: send to session %s: %v", sessionID, err)
			continue
		}
		if m.cfg.ConnectIPDatagramsSent != nil {
			m.cfg.ConnectIPDatagramsSent.Inc()
		}
	}
}

func (m *sharedTunManager) registerSession(id string, send func([]byte) error) (deregister func()) {
	m.mu.Lock()
	m.sessions[id] = &sharedTunSession{id: id, send: send}
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.sessions, id)
		for ip, sid := range m.ipToSession {
			if sid.sessionID == id {
				delete(m.ipToSession, ip)
			}
		}
		m.mu.Unlock()
	}
}

func (m *sharedTunManager) bindClientIP(id, srcIP string) {
	if srcIP == "" {
		return
	}
	m.mu.Lock()
	now := time.Now()
	if old, ok := m.ipToSession[srcIP]; ok && old.sessionID != "" && old.sessionID != id {
		if m.cfg.ConnectIPTunSharedBindingConflicts != nil {
			m.cfg.ConnectIPTunSharedBindingConflicts.Inc()
		}
	}
	m.ipToSession[srcIP] = sharedTunBinding{sessionID: id, lastSeen: now}
	m.mu.Unlock()
}

func (m *sharedTunManager) bindingTTL() time.Duration {
	if m.cfg.ConnectIPTunSharedBindingTTL > 0 {
		return m.cfg.ConnectIPTunSharedBindingTTL
	}
	return 5 * time.Minute
}

func (m *sharedTunManager) gcLoop() {
	ttl := m.bindingTTL()
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()
	for range tk.C {
		cutoff := time.Now().Add(-ttl)
		evicted := 0
		m.mu.Lock()
		for ip, b := range m.ipToSession {
			if b.lastSeen.Before(cutoff) {
				delete(m.ipToSession, ip)
				evicted++
			}
		}
		m.mu.Unlock()
		if evicted > 0 && m.cfg.ConnectIPTunSharedBindingStaleEvictions != nil {
			m.cfg.ConnectIPTunSharedBindingStaleEvictions.Add(float64(evicted))
		}
	}
}

func runConnectIPSharedTunSessionLoop(ctx context.Context, str *http3.Stream, cfg ListenConfig, acl map[string]any) {
	mgr, err := getOrCreateSharedTunManager(cfg)
	if err != nil {
		if cfg.ConnectIPTunOpenEchoFallbacks != nil {
			cfg.ConnectIPTunOpenEchoFallbacks.Inc()
		}
		log.Printf("connect-ip: shared TUN unavailable: %v; echo mode", err)
		runConnectIPDatagramEchoOnlyLoop(ctx, str, cfg, acl)
		return
	}

	sessionID := fmt.Sprintf("%p", str)
	var sendMu sync.Mutex
	send := func(frame []byte) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return str.SendDatagram(frame)
	}
	deregister := mgr.registerSession(sessionID, send)
	defer deregister()
	if cfg.ConnectIPTunBridgeActive != nil {
		cfg.ConnectIPTunBridgeActive.Inc()
		defer cfg.ConnectIPTunBridgeActive.Dec()
	}

	for {
		data, err := str.ReceiveDatagram(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("connect-ip shared tun: datagram recv end remote err=%v", err)
			return
		}
		if len(data) == 0 || len(data) > maxConnectIPDatagramBytes {
			if cfg.ConnectIPDatagramDrops != nil {
				cfg.ConnectIPDatagramDrops.Inc()
			}
			continue
		}
		if cfg.ConnectIPDatagramsReceived != nil {
			cfg.ConnectIPDatagramsReceived.Inc()
		}
		payload, drop, unknownCtx := rfc9484ConnectIPDatagramPayloadForPolicy(data, cfg.ConnectIPEchoContextIDs)
		if unknownCtx {
			if cfg.ConnectIPDatagramUnknownContext != nil {
				cfg.ConnectIPDatagramUnknownContext.Inc()
			}
			continue
		}
		if drop {
			if cfg.ConnectIPDatagramDrops != nil {
				cfg.ConnectIPDatagramDrops.Inc()
			}
			continue
		}
		if src, _, ok := parseConnectIPDatagramEndpoints(payload); ok && src != nil {
			mgr.bindClientIP(sessionID, src.String())
		}
		if err := processInboundConnectIPDatagram(ctx, str, cfg, acl, data, mgr.tun); err != nil {
			log.Printf("connect-ip shared tun: %v", err)
			return
		}
	}
}
