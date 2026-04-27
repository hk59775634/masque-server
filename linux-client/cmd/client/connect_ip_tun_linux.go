//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
	"golang.org/x/sys/unix"
)

// cmdConnectIPTun creates a Linux TUN interface and bridges IP frames to CONNECT-IP HTTP/3 datagrams (RFC 9484 Context ID 0).
// The masque-server stub echoes datagrams; this is for protocol/TUN plumbing. Real routing needs a forwarding server.
func cmdConnectIPTun(args []string) {
	fs := flag.NewFlagSet("connect-ip-tun", flag.ExitOnError)
	var masqueURL, connectIPUDP, tunName, addrCIDR string
	noAddrCapsule := fs.Bool("no-address-capsule", false, "do not send ADDRESS_REQUEST or apply ADDRESS_ASSIGN from the CONNECT-IP stream (use with -addr only)")
	applyRoutesFromCapsule := fs.Bool("apply-routes-from-capsule", false, "apply RFC 9484 ROUTE_ADVERTISEMENT to system routes (IPv4 single-CIDR ranges only; uses ip route replace)")
	fs.StringVar(&masqueURL, "masque-server", "", "MASQUE server base URL (default: from config)")
	fs.StringVar(&connectIPUDP, "connect-ip-udp", "", "override UDP host:port (default: from GET /v1/masque/capabilities listen_udp)")
	fs.StringVar(&tunName, "tun-name", "", "requested TUN interface name (empty = kernel assigns, e.g. tun0)")
	fs.StringVar(&addrCIDR, "addr", "", "optional: run \"ip addr add <cidr> dev <if>\" after bring-up (requires CAP_NET_ADMIN / root); when unset, sends ADDRESS_REQUEST and applies server ADDRESS_ASSIGN (stub e.g. 192.0.2.1/32)")
	mtu := fs.Int("mtu", 1280, "set interface MTU via ip-link (0 = skip)")
	linkUp := fs.Bool("up", true, "run \"ip link set dev <if> up\"")
	reconnect := fs.Bool("reconnect", true, "auto redial CONNECT-IP when stream/datagram path breaks")
	reconnectInitialBackoff := fs.Duration("reconnect-initial-backoff", 1*time.Second, "initial backoff before reconnect retry")
	reconnectMaxBackoff := fs.Duration("reconnect-max-backoff", 15*time.Second, "max backoff between reconnect retries")
	reconnectMaxDialFailures := fs.Int("reconnect-max-dial-failures", 0, "exit after this many consecutive dial failures (0 = unlimited; resets after a successful dial)")
	reconnectLogInterval := fs.Duration("reconnect-log-interval", 30*time.Second, "min interval between similar reconnect logs (0 = log every event)")
	splitDefaultRoute := fs.Bool("split-default-route", false, "IPv4: install split default (0.0.0.0/1 + 128.0.0.0/1) via TUN (per §7.3 global-style routing); prefer -route split|all")
	var routeMode string
	fs.StringVar(&routeMode, "route", "", "IPv4 routing: empty (honour -split-default-route only), none, split, or all (all = split)")
	var dnsCSV string
	fs.StringVar(&dnsCSV, "dns", "", "comma-separated resolvers; overwrites /etc/resolv.conf with backup (root); restored on exit")
	dnsResolvectl := fs.Bool("dns-resolvectl", false, "with -dns: use `resolvectl dns` on the IPv4 default-route interface and `resolvectl revert` on exit (systemd-resolved); needs resolvectl and typically root")
	dnsResolvectlFallback := fs.Bool("dns-resolvectl-fallback", true, "with -dns -dns-resolvectl: if resolvectl fails, fall back to overwriting /etc/resolv.conf (same as without -dns-resolvectl); when false, exit on resolvectl failure")
	bypassMasqueHost := fs.Bool("bypass-masque-host", true, "with -route split|all (or -split-default-route): add /32 for QUIC server (and masque HTTPS host if different) via system default gateway (anti black-hole)")
	reconnectMaxSessionDrops := fs.Int("reconnect-max-session-drops", 0, "exit after this many consecutive session drops after a successful dial (0 = unlimited; resets on each successful dial)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client connect-ip-tun [-masque-server URL] [-connect-ip-udp host:port] [-tun-name NAME] [-addr CIDR] [-no-address-capsule] [-apply-routes-from-capsule] [-route none|split|all] [-split-default-route] [-dns 1.1.1.1,8.8.8.8] [-dns-resolvectl] [-dns-resolvectl-fallback=true] [-bypass-masque-host=true] [-mtu N] [-up=false] [-reconnect] [-reconnect-initial-backoff 1s] [-reconnect-max-backoff 15s] [-reconnect-max-dial-failures N] [-reconnect-max-session-drops N] [-reconnect-log-interval 30s]\n")
		fmt.Fprintf(os.Stderr, "  Linux only. Opens CONNECT-IP, creates TUN, maps TUN <-> RFC 9484 CID0 datagrams. Ctrl+C to exit.\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	dnsList := parseCommaList(dnsCSV)
	routeTrim := strings.TrimSpace(strings.ToLower(routeMode))
	var doSplitRoutes bool
	switch routeTrim {
	case "":
		doSplitRoutes = *splitDefaultRoute
	case "none":
		if *splitDefaultRoute {
			log.Printf("connect-ip-tun: warn: -split-default-route ignored because -route=none")
		}
		doSplitRoutes = false
	case "split", "all":
		doSplitRoutes = true
	default:
		fmt.Fprintf(os.Stderr, "error: -route must be empty, none, split, or all\n")
		os.Exit(1)
	}

	cfg, cfgLoaded := ClientConfig{}, false
	if c, err := loadConfig(); err == nil {
		cfg = c
		cfgLoaded = true
	}
	if masqueURL == "" && cfgLoaded {
		masqueURL = strings.TrimSpace(cfg.MasqueServerURL)
	}
	masqueURL = strings.TrimRight(strings.TrimSpace(masqueURL), "/")
	if masqueURL == "" {
		fmt.Fprintf(os.Stderr, "error: set masque_server_url in config or pass -masque-server\n")
		os.Exit(1)
	}
	if !cfgLoaded || strings.TrimSpace(cfg.DeviceToken) == "" || strings.TrimSpace(cfg.Fingerprint) == "" {
		fmt.Fprintf(os.Stderr, "error: need config with device_token and fingerprint (client activate)\n")
		os.Exit(1)
	}

	var udpTarget string
	var udpErr error
	if s := strings.TrimSpace(connectIPUDP); s != "" {
		udpTarget = s
	} else {
		capURL := joinURL(masqueURL, "/v1/masque/capabilities")
		client := &http.Client{Timeout: 8 * time.Second}
		raw, code, err := httpGetBody(context.Background(), client, capURL)
		if err != nil || code != http.StatusOK {
			fmt.Fprintf(os.Stderr, "error: need GET %s -> 200 to read listen_udp, or pass -connect-ip-udp: http=%d err=%v\n", capURL, code, err)
			os.Exit(1)
		}
		udpTarget, udpErr = quicUDPAddrFromCapabilities(raw, masqueURL)
	}
	if udpErr != nil || strings.TrimSpace(udpTarget) == "" {
		fmt.Fprintf(os.Stderr, "error: UDP target: %v\n", udpErr)
		os.Exit(1)
	}

	if len(dnsList) > 0 && os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "error: -dns requires root to write /etc/resolv.conf\n")
		os.Exit(1)
	}
	if doSplitRoutes || *applyRoutesFromCapsule || strings.TrimSpace(addrCIDR) != "" {
		if os.Geteuid() != 0 && !hasCapNetAdminLinux() {
			fmt.Fprintf(os.Stderr, "error: need root or CAP_NET_ADMIN for -route/-split-default-route/-apply-routes-from-capsule/-addr\n")
			os.Exit(1)
		}
	}

	tunFile, ifName, err := openLinuxTUN(tunName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open TUN: %v\n", err)
		os.Exit(1)
	}

	var routeCleanup, dnsRestore func()
	defer func() {
		if routeCleanup != nil {
			routeCleanup()
		}
		if dnsRestore != nil {
			dnsRestore()
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	wantAutoAddr := !*noAddrCapsule && strings.TrimSpace(addrCIDR) == ""

	if *linkUp {
		if err := runIP("link", "set", "dev", ifName, "up"); err != nil {
			_ = tunFile.Close()
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	if *mtu > 0 {
		if err := runIP("link", "set", "dev", ifName, "mtu", fmt.Sprintf("%d", *mtu)); err != nil {
			log.Printf("warn: set mtu: %v", err)
		}
	}
	if s := strings.TrimSpace(addrCIDR); s != "" {
		if err := runIP("addr", "add", s, "dev", ifName); err != nil {
			log.Printf("warn: ip addr add: %v", err)
		}
	}

	if len(dnsList) > 0 {
		var restore func()
		var derr error
		if *dnsResolvectl {
			restore, derr = applyTunDNSViaResolvectl(dnsList)
			if derr != nil {
				if !*dnsResolvectlFallback {
					_ = tunFile.Close()
					fmt.Fprintf(os.Stderr, "error: -dns -dns-resolvectl: %v (use -dns-resolvectl-fallback=true to allow /etc/resolv.conf fallback)\n", derr)
					os.Exit(1)
				}
				log.Printf("connect-ip-tun: warn: -dns-resolvectl failed (%v); falling back to /etc/resolv.conf", derr)
				restore, derr = applyTunDNS(dnsList)
				if derr != nil {
					_ = tunFile.Close()
					fmt.Fprintf(os.Stderr, "error: -dns after resolvectl fallback: %v\n", derr)
					os.Exit(1)
				}
				dnsRestore = restore
				log.Printf("connect-ip-tun: wrote /etc/resolv.conf with %d nameserver(s) (resolvectl fallback)", len(dnsList))
			} else {
				dnsRestore = restore
				log.Printf("connect-ip-tun: applied -dns via resolvectl (%d nameserver(s))", len(dnsList))
			}
		} else {
			restore, derr = applyTunDNS(dnsList)
			if derr != nil {
				_ = tunFile.Close()
				fmt.Fprintf(os.Stderr, "error: -dns: %v\n", derr)
				os.Exit(1)
			}
			dnsRestore = restore
			log.Printf("connect-ip-tun: wrote /etc/resolv.conf with %d nameserver(s)", len(dnsList))
		}
	}

	if doSplitRoutes {
		cleanup, rerr := installSplitDefaultAndBypass(ifName, udpTarget, masqueURL, *bypassMasqueHost)
		if rerr != nil {
			_ = tunFile.Close()
			fmt.Fprintf(os.Stderr, "error: split default / bypass routes: %v\n", rerr)
			os.Exit(1)
		}
		routeCleanup = cleanup
		log.Printf("connect-ip-tun: installed IPv4 split-default via %s (bypass-masque-host=%v)", ifName, *bypassMasqueHost)
	}

	fmt.Printf("connect-ip-tun: if=%s udp=%s (Ctrl+C to stop)\n", ifName, udpTarget)

	if *reconnectInitialBackoff <= 0 {
		*reconnectInitialBackoff = time.Second
	}
	if *reconnectMaxBackoff < *reconnectInitialBackoff {
		*reconnectMaxBackoff = *reconnectInitialBackoff
	}

	packetCh := make(chan []byte, 256)
	tunReadErrCh := make(chan error, 1)
	go readTUNPackets(ctx, tunFile, packetCh, tunReadErrCh)

	retryBackoff := *reconnectInitialBackoff
	autoAddrApplied := false
	dialThrottle := &logThrottle{minInterval: *reconnectLogInterval}
	sessionThrottle := &logThrottle{minInterval: *reconnectLogInterval}
	consecutiveDialFailures := 0
	consecutiveSessionDrops := 0
runLoop:
	for {
		if ctx.Err() != nil {
			break
		}
		dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
		sess, err := dialConnectIP(dctx, udpTarget, cfg.DeviceToken, cfg.Fingerprint)
		dcancel()
		if err != nil {
			if !*reconnect {
				_ = tunFile.Close()
				fmt.Fprintf(os.Stderr, "error: CONNECT-IP: %v\n", err)
				os.Exit(1)
			}
			consecutiveDialFailures++
			if *reconnectMaxDialFailures > 0 && consecutiveDialFailures >= *reconnectMaxDialFailures {
				_ = tunFile.Close()
				fmt.Fprintf(os.Stderr, "error: CONNECT-IP: gave up after %d consecutive dial failures (last: %v)\n", consecutiveDialFailures, err)
				os.Exit(1)
			}
			dialThrottle.maybeLog(func(suppressed int) {
				if suppressed > 0 {
					log.Printf("connect-ip-tun: dial failed: %v; reconnect in %s (%d similar since last log)", err, retryBackoff, suppressed)
					return
				}
				log.Printf("connect-ip-tun: dial failed: %v; reconnect in %s", err, retryBackoff)
			})
			if !sleepWithContext(ctx, retryBackoff) {
				break
			}
			retryBackoff = nextBackoff(retryBackoff, *reconnectMaxBackoff)
			continue
		}
		consecutiveDialFailures = 0
		consecutiveSessionDrops = 0
		dialThrottle.reset()
		sessionThrottle.reset()
		log.Printf("connect-ip-tun: connected to %s", udpTarget)
		retryBackoff = *reconnectInitialBackoff

		wantAutoAddrThisSession := wantAutoAddr && !autoAddrApplied
		assignCh := make(chan string, 1)
		capsuleErrCh := make(chan error, 1)
		go func() {
			capsuleErrCh <- drainConnectIPCapsules(sess.Resp.Body, ifName, wantAutoAddrThisSession, *applyRoutesFromCapsule, assignCh)
		}()
		if wantAutoAddrThisSession {
			req := encodeAddressRequestIPv4Unspecified(connectIPCapsuleRequestID())
			if _, err := sess.RS.Write(req); err != nil {
				log.Printf("connect-ip-tun: warn: ADDRESS_REQUEST write: %v", err)
			}
		}

		sessionErr := runConnectIPSession(ctx, tunFile, sess.RS, packetCh, capsuleErrCh)
		select {
		case <-assignCh:
			autoAddrApplied = true
		default:
		}
		sess.Close()

		if ctx.Err() != nil {
			break
		}
		select {
		case terr := <-tunReadErrCh:
			if terr != nil && !errors.Is(terr, os.ErrClosed) && !errors.Is(terr, unix.EBADF) {
				log.Printf("connect-ip-tun: TUN reader stopped: %v", terr)
			}
			break runLoop
		default:
		}
		if sessionErr != nil && !errors.Is(sessionErr, context.Canceled) {
			consecutiveSessionDrops++
			if *reconnectMaxSessionDrops > 0 && consecutiveSessionDrops >= *reconnectMaxSessionDrops {
				_ = tunFile.Close()
				fmt.Fprintf(os.Stderr, "error: CONNECT-IP: gave up after %d consecutive session drops (last: %v)\n", consecutiveSessionDrops, sessionErr)
				os.Exit(1)
			}
			sessionThrottle.maybeLog(func(suppressed int) {
				if suppressed > 0 {
					log.Printf("connect-ip-tun: session ended: %v (%d similar since last log)", sessionErr, suppressed)
					return
				}
				log.Printf("connect-ip-tun: session ended: %v", sessionErr)
			})
		}
		if !*reconnect {
			break
		}
		if !sleepWithContext(ctx, retryBackoff) {
			break
		}
		retryBackoff = nextBackoff(retryBackoff, *reconnectMaxBackoff)
	}
	_ = tunFile.Close()
	fmt.Println("connect-ip-tun: stopped")
}

func readTUNPackets(ctx context.Context, tunFile *os.File, out chan<- []byte, errCh chan<- error) {
	buf := make([]byte, 65536)
	for {
		n, err := tunFile.Read(buf)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		if n <= 0 {
			continue
		}
		pkt := append([]byte(nil), buf[:n]...)
		select {
		case out <- pkt:
		default:
			// Drop when disconnected/backlogged to avoid unbounded memory growth.
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func runConnectIPSession(ctx context.Context, tunFile *os.File, rs *http3.RequestStream, packetCh <-chan []byte, capsuleErrCh <-chan error) error {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 3)
	go func() {
		for {
			select {
			case <-sessCtx.Done():
				return
			case pkt := <-packetCh:
				frame := rfc9484PrependContext0(pkt)
				if err := rs.SendDatagram(frame); err != nil {
					errCh <- fmt.Errorf("send datagram: %w", err)
					return
				}
			}
		}
	}()
	go func() {
		for {
			dg, err := rs.ReceiveDatagram(sessCtx)
			if err != nil {
				errCh <- fmt.Errorf("receive datagram: %w", err)
				return
			}
			inner, err := rfc9484StripContext0(dg)
			if err != nil {
				log.Printf("drop datagram: %v", err)
				continue
			}
			if len(inner) == 0 {
				continue
			}
			if _, err := tunFile.Write(inner); err != nil {
				errCh <- fmt.Errorf("write tun: %w", err)
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-capsuleErrCh:
		if err == nil {
			return fmt.Errorf("capsule stream closed")
		}
		return fmt.Errorf("capsule stream: %w", err)
	case err := <-errCh:
		return err
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	if cur >= max {
		return max
	}
	n := cur * 2
	if n > max {
		return max
	}
	return n
}

// logThrottle coalesces repeated log lines (e.g. during long outages).
type logThrottle struct {
	mu          sync.Mutex
	minInterval time.Duration
	lastEmit    time.Time
	suppressed  int
}

func (t *logThrottle) reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastEmit = time.Time{}
	t.suppressed = 0
}

func (t *logThrottle) maybeLog(emit func(suppressed int)) {
	if t == nil {
		emit(0)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.minInterval <= 0 {
		emit(0)
		return
	}
	now := time.Now()
	if t.lastEmit.IsZero() || now.Sub(t.lastEmit) >= t.minInterval {
		sup := t.suppressed
		t.suppressed = 0
		emit(sup)
		t.lastEmit = now
		return
	}
	t.suppressed++
}

func openLinuxTUN(requestedName string) (*os.File, string, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR, 0)
	if err != nil {
		return nil, "", fmt.Errorf("open /dev/net/tun: %w", err)
	}
	ifr, err := unix.NewIfreq(strings.TrimSpace(requestedName))
	if err != nil {
		unix.Close(fd)
		return nil, "", err
	}
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err := unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr); err != nil {
		unix.Close(fd)
		return nil, "", fmt.Errorf("TUNSETIFF: %w", err)
	}
	ifName := ifr.Name()
	return os.NewFile(uintptr(fd), ifName), ifName, nil
}

func runIP(args ...string) error {
	cmd := exec.Command("ip", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip %v: %w", args, err)
	}
	return nil
}
