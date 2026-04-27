//go:build linux

package main

import (
	"context"
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
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client connect-ip-tun [-masque-server URL] [-connect-ip-udp host:port] [-tun-name NAME] [-addr CIDR] [-no-address-capsule] [-apply-routes-from-capsule] [-mtu N] [-up=false]\n")
		fmt.Fprintf(os.Stderr, "  Linux only. Opens CONNECT-IP, creates TUN, maps TUN <-> RFC 9484 CID0 datagrams. Ctrl+C to exit.\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

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

	tunFile, ifName, err := openLinuxTUN(tunName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open TUN: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
	sess, err := dialConnectIP(dctx, udpTarget, cfg.DeviceToken, cfg.Fingerprint)
	dcancel()
	if err != nil {
		_ = tunFile.Close()
		fmt.Fprintf(os.Stderr, "error: CONNECT-IP: %v\n", err)
		os.Exit(1)
	}
	defer sess.Close()

	wantAutoAddr := !*noAddrCapsule && strings.TrimSpace(addrCIDR) == ""
	if wantAutoAddr {
		req := encodeAddressRequestIPv4Unspecified(connectIPCapsuleRequestID())
		if _, err := sess.RS.Write(req); err != nil {
			log.Printf("connect-ip-tun: warn: ADDRESS_REQUEST write: %v", err)
		}
	}
	go func() {
		if err := drainConnectIPCapsules(sess.Resp.Body, ifName, wantAutoAddr, *applyRoutesFromCapsule, nil); err != nil {
			log.Printf("connect-ip-tun: capsule reader finished: %v", err)
		}
	}()

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

	fmt.Printf("connect-ip-tun: if=%s udp=%s (Ctrl+C to stop)\n", ifName, udpTarget)

	rs := sess.RS
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 65536)
		for {
			n, err := tunFile.Read(buf)
			if err != nil {
				return
			}
			if n <= 0 {
				continue
			}
			frame := rfc9484PrependContext0(append([]byte(nil), buf[:n]...))
			if err := rs.SendDatagram(frame); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			dg, err := rs.ReceiveDatagram(ctx)
			if err != nil {
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
				return
			}
		}
	}()

	<-ctx.Done()
	_ = tunFile.Close()
	wg.Wait()
	fmt.Println("connect-ip-tun: stopped")
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
