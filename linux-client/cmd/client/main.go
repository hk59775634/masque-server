package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Set at link time: -X main.version=... -X main.commit=... -X main.date=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type ClientConfig struct {
	ControlPlaneURL string   `json:"control_plane_url"`
	MasqueServerURL string   `json:"masque_server_url"`
	Fingerprint     string   `json:"fingerprint"`
	DeviceToken     string   `json:"device_token"`
	DeviceID        int      `json:"device_id"`
	Routes          []string `json:"routes"`
	DNS             []string `json:"dns"`
}

type activateResponse struct {
	DeviceID    int    `json:"device_id"`
	DeviceToken string `json:"device_token"`
	Config      struct {
		ServerAddr string   `json:"server_addr"`
		Routes     []string `json:"routes"`
		DNS        []string `json:"dns"`
	} `json:"config"`
}

type connectResponse struct {
	Session     string   `json:"session"`
	Routes      []string `json:"routes"`
	DNS         []string `json:"dns"`
	PolicyACL   any      `json:"policy_acl"`
	MasqueReady bool     `json:"masque_ready"`
}

type RuntimeState struct {
	AddedRoutes      []string `json:"added_routes"`
	ResolvConfBackup string   `json:"resolv_conf_backup"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "activate":
		cmdActivate(os.Args[2:])
	case "connect":
		cmdConnect(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "disconnect":
		cmdDisconnect(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "version":
		cmdVersion()
	case "config":
		if len(os.Args) < 3 {
			usageConfig()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "show":
			cmdConfigShow(os.Args[3:])
		case "path":
			cmdConfigPath()
		case "export":
			cmdConfigExport(os.Args[3:])
		case "import":
			cmdConfigImport(os.Args[3:])
		default:
			usageConfig()
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func cmdActivate(args []string) {
	fs := flag.NewFlagSet("activate", flag.ExitOnError)
	var cpURL, fingerprint, code string
	verify := fs.Bool("verify", false, "probe control plane before POST /activate; after success probe masque /healthz (warn if masque down; config still saved)")
	fs.StringVar(&cpURL, "control-plane", "http://127.0.0.1:8000", "control plane URL")
	fs.StringVar(&fingerprint, "fingerprint", "", "device fingerprint")
	fs.StringVar(&code, "code", "", "activation code")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client activate [-control-plane URL] -fingerprint ... -code ... [-verify]\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if fingerprint == "" || code == "" {
		fatal(errors.New("-fingerprint and -code are required"))
	}

	cpBase := strings.TrimRight(strings.TrimSpace(cpURL), "/")

	if *verify {
		client := &http.Client{Timeout: 8 * time.Second}
		ctx := context.Background()
		if _, err := checkControlPlaneReachable(ctx, client, cpBase); err != nil {
			fatal(fmt.Errorf("activate -verify: control plane: %w", err))
		}
		fmt.Fprintln(os.Stderr, "activate -verify: control plane OK")
	}

	reqBody := map[string]string{"activation_code": code, "fingerprint": fingerprint}
	raw, err := postJSON(joinURL(cpBase, "/api/v1/activate"), reqBody)
	if err != nil {
		fatal(err)
	}

	var resp activateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		fatal(err)
	}

	cfg := ClientConfig{
		ControlPlaneURL: cpBase,
		MasqueServerURL: "http://127.0.0.1:8443",
		Fingerprint:     fingerprint,
		DeviceToken:     resp.DeviceToken,
		DeviceID:        resp.DeviceID,
	}
	if resp.Config.ServerAddr != "" {
		cfg.MasqueServerURL = resp.Config.ServerAddr
	}
	cfg.Routes = resp.Config.Routes
	cfg.DNS = resp.Config.DNS

	if *verify {
		client := &http.Client{Timeout: 8 * time.Second}
		ctx := context.Background()
		mHealth := joinURL(strings.TrimRight(cfg.MasqueServerURL, "/"), "/healthz")
		httpCode, errHealth := httpGetStatus(ctx, client, mHealth)
		if errHealth != nil || httpCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "activate -verify: warning: masque_server GET %s (err=%v, status=%d); config still saved\n", mHealth, errHealth, httpCode)
		} else {
			fmt.Fprintln(os.Stderr, "activate -verify: masque_server OK")
		}
	}

	if err := saveConfig(cfg); err != nil {
		fatal(err)
	}

	fmt.Printf("activated: device_id=%d\n", resp.DeviceID)
}

func cmdConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	doCheck := fs.Bool("check", false, "verify GET /api/v1/devices/self on control plane before connecting to masque")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client connect [-check]\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	if *doCheck {
		if err := assertControlPlaneDeviceSelf(fetchDeviceSelfRemote(cfg)); err != nil {
			fatal(err)
		}
		fmt.Fprintln(os.Stderr, "connect: control plane device/policy OK (-check)")
	}

	reqBody := map[string]string{"device_token": cfg.DeviceToken, "fingerprint": cfg.Fingerprint}
	raw, err := postJSON(cfg.MasqueServerURL+"/connect", reqBody)
	if err != nil {
		fatal(err)
	}

	var resp connectResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		fatal(err)
	}
	if len(resp.Routes) == 0 {
		resp.Routes = cfg.Routes
	}
	if len(resp.DNS) == 0 {
		resp.DNS = cfg.DNS
	}

	state := RuntimeState{}
	if err := applyRoutes(resp.Routes, &state); err != nil {
		fatal(err)
	}
	if err := applyDNS(resp.DNS, &state); err != nil {
		_ = restoreRuntime(state)
		fatal(err)
	}
	if err := saveRuntimeState(state); err != nil {
		fatal(err)
	}

	fmt.Printf("connected: session=%s routes=%v dns=%v\n", resp.Session, resp.Routes, resp.DNS)
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	live := fs.Bool("live", false, "query control plane GET /api/v1/devices/self (Bearer device token)")
	jsonOut := fs.Bool("json", false, "print JSON; add -live to include control plane device/policy")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client status [-live] [-json]\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	if *jsonOut {
		local := map[string]any{
			"device_id":             cfg.DeviceID,
			"fingerprint":           cfg.Fingerprint,
			"control_plane_url":     cfg.ControlPlaneURL,
			"masque_server_url":     cfg.MasqueServerURL,
			"routes":                cfg.Routes,
			"dns":                   cfg.DNS,
			"device_token_redacted": redactDeviceToken(cfg.DeviceToken),
		}
		out := map[string]any{"local": local}
		if *live {
			out["remote"] = fetchDeviceSelfRemote(cfg)
		}
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(raw))
		return
	}

	fmt.Printf("device_id=%d fingerprint=%s control_plane=%s masque_server=%s routes=%v dns=%v\n", cfg.DeviceID, cfg.Fingerprint, cfg.ControlPlaneURL, cfg.MasqueServerURL, cfg.Routes, cfg.DNS)

	if *live {
		if strings.TrimSpace(cfg.DeviceToken) == "" {
			fmt.Fprintln(os.Stderr, "status -live: device_token empty, skipping control plane")
			return
		}
		remote := fetchDeviceSelfRemote(cfg)
		fmt.Println("--- control plane GET /api/v1/devices/self ---")
		switch v := remote.(type) {
		case map[string]any:
			if errMsg, ok := v["error"].(string); ok {
				fmt.Fprintf(os.Stderr, "error: %s\n", errMsg)
				return
			}
			if scRaw, ok := v["http_status"]; ok && !httpStatusIs200(scRaw) {
				raw, _ := json.MarshalIndent(v, "", "  ")
				fmt.Println(string(raw))
				return
			}
			raw, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				fatal(err)
			}
			fmt.Println(string(raw))
		default:
			raw, _ := json.MarshalIndent(remote, "", "  ")
			fmt.Println(string(raw))
		}
	}
}

func httpStatusIs200(v any) bool {
	switch x := v.(type) {
	case int:
		return x == http.StatusOK
	case int64:
		return int(x) == http.StatusOK
	case float64:
		return int(x) == http.StatusOK
	default:
		return false
	}
}

func fetchDeviceSelfRemote(cfg ClientConfig) any {
	if strings.TrimSpace(cfg.DeviceToken) == "" {
		return map[string]string{"skipped": "empty device_token"}
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.ControlPlaneURL), "/")
	url := joinURL(base, "/api/v1/devices/self")
	raw, code, err := httpGetBearer(url, cfg.DeviceToken, 8*time.Second)
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	if code != http.StatusOK {
		return map[string]any{
			"http_status": code,
			"body":        string(raw),
		}
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return map[string]string{"error": "invalid json: " + err.Error(), "body": string(raw)}
	}
	return parsed
}

func assertControlPlaneDeviceSelf(remote any) error {
	m, ok := remote.(map[string]any)
	if !ok {
		return fmt.Errorf("control plane: unexpected response type %T", remote)
	}
	if s, ok := m["skipped"].(string); ok {
		return fmt.Errorf("control plane: %s", s)
	}
	if errStr, ok := m["error"].(string); ok {
		return fmt.Errorf("control plane: %s", errStr)
	}
	if scRaw, ok := m["http_status"]; ok && !httpStatusIs200(scRaw) {
		b, _ := m["body"].(string)
		return fmt.Errorf("control plane: HTTP %v: %s", scRaw, strings.TrimSpace(b))
	}
	if _, ok := m["device"]; !ok {
		return errors.New("control plane: response missing device")
	}
	return nil
}

func httpGetBearer(url, bearer string, timeout time.Duration) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearer))
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	var cpURL, masqueURL string
	fs.StringVar(&cpURL, "control-plane", "", "control plane base URL (default: from config or http://127.0.0.1:8000)")
	fs.StringVar(&masqueURL, "masque-server", "", "MASQUE server base URL (default: from config; omit check if unset and no config)")
	strict := fs.Bool("strict", false, "require masque_server URL and a successful /healthz check (no SKIP)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client doctor [-control-plane URL] [-masque-server URL] [-strict]\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	var cfg ClientConfig
	cfgLoaded := false
	if c, err := loadConfig(); err == nil {
		cfg = c
		cfgLoaded = true
	}

	if cpURL == "" {
		if cfgLoaded && strings.TrimSpace(cfg.ControlPlaneURL) != "" {
			cpURL = strings.TrimSpace(cfg.ControlPlaneURL)
		} else {
			cpURL = "http://127.0.0.1:8000"
		}
	}
	cpURL = strings.TrimRight(strings.TrimSpace(cpURL), "/")

	if masqueURL == "" && cfgLoaded {
		masqueURL = strings.TrimSpace(cfg.MasqueServerURL)
	}
	masqueURL = strings.TrimRight(strings.TrimSpace(masqueURL), "/")

	client := &http.Client{Timeout: 8 * time.Second}
	ctx := context.Background()

	fail := 0
	warn := 0

	printCheck := func(level, name, detail string) {
		fmt.Printf("[%s] %s: %s\n", level, name, detail)
	}

	if _, err := exec.LookPath("ip"); err != nil {
		printCheck("FAIL", "ip", "not found in PATH (required for connect route changes)")
		fail++
	} else {
		printCheck("OK", "ip", "found in PATH")
	}

	if runtime.GOOS == "linux" {
		if os.Geteuid() == 0 {
			printCheck("OK", "privileges", "running as root (connect/disconnect can change routes and /etc/resolv.conf)")
		} else if hasCapNetAdminLinux() {
			printCheck("OK", "privileges", "CAP_NET_ADMIN present (non-root route changes may work)")
		} else {
			printCheck("WARN", "privileges", "not root and no CAP_NET_ADMIN; connect will likely fail when applying routes or DNS")
			warn++
		}
	} else {
		printCheck("WARN", "privileges", fmt.Sprintf("GOOS=%s; expect Linux for production use", runtime.GOOS))
		warn++
	}

	if !cfgLoaded {
		printCheck("WARN", "config", fmt.Sprintf("missing or unreadable (%s); activate first", configPath()))
		warn++
	} else {
		printCheck("OK", "config", fmt.Sprintf("loaded from %s", configPath()))
		if strings.TrimSpace(cfg.DeviceToken) == "" {
			printCheck("FAIL", "config.device_token", "empty")
			fail++
		} else {
			printCheck("OK", "config.device_token", "present")
		}
		if strings.TrimSpace(cfg.Fingerprint) == "" {
			printCheck("FAIL", "config.fingerprint", "empty")
			fail++
		} else {
			printCheck("OK", "config.fingerprint", "present")
		}
	}

	if okURL, err := checkControlPlaneReachable(ctx, client, cpURL); err != nil {
		printCheck("FAIL", "control_plane", err.Error())
		fail++
	} else {
		printCheck("OK", "control_plane", fmt.Sprintf("GET %s -> 200", okURL))
	}

	if masqueURL == "" {
		if *strict {
			printCheck("FAIL", "masque_server", "strict: no URL (set masque_server_url in config after activate, or pass -masque-server)")
			fail++
		} else {
			printCheck("SKIP", "masque_server", "no URL (set in config after activate, or pass -masque-server; use -strict to require)")
		}
	} else {
		mHealth := joinURL(masqueURL, "/healthz")
		if code, err := httpGetStatus(ctx, client, mHealth); err != nil {
			printCheck("FAIL", "masque_server", fmt.Sprintf("GET %s: %v", mHealth, err))
			fail++
		} else if code != http.StatusOK {
			printCheck("FAIL", "masque_server", fmt.Sprintf("GET %s returned %d", mHealth, code))
			fail++
		} else {
			printCheck("OK", "masque_server", fmt.Sprintf("GET %s -> 200", mHealth))
		}
	}

	fmt.Printf("summary: fail=%d warn=%d\n", fail, warn)
	if fail > 0 {
		os.Exit(1)
	}
}

func hasCapNetAdminLinux() bool {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "CapEff:\t") {
			hexStr := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:\t"))
			var caps uint64
			_, err := fmt.Sscanf(hexStr, "%x", &caps)
			if err != nil {
				return false
			}
			const capNetAdmin = 1 << 12
			return caps&capNetAdmin != 0
		}
	}
	return false
}

func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func httpGetStatus(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func checkControlPlaneReachable(ctx context.Context, client *http.Client, base string) (string, error) {
	candidates := []string{
		joinURL(base, "/api/v1/health"),
		joinURL(base, "/up"),
	}
	var errs []string
	for _, u := range candidates {
		code, err := httpGetStatus(ctx, client, u)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", u, err))
			continue
		}
		if code == http.StatusOK {
			return u, nil
		}
		errs = append(errs, fmt.Sprintf("%s -> %d", u, code))
	}
	return "", fmt.Errorf("tried %s", strings.Join(errs, "; "))
}

func cmdDisconnect(_ []string) {
	state, err := loadRuntimeState()
	if err != nil {
		fatal(err)
	}
	if err := restoreRuntime(state); err != nil {
		fatal(err)
	}
	_ = os.Remove(statePath())
	fmt.Println("disconnect: routes and dns restored")
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".masque-client.json"
	}
	return filepath.Join(home, ".masque-client.json")
}

func statePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".masque-client-state.json"
	}
	return filepath.Join(home, ".masque-client-state.json")
}

func saveConfig(cfg ClientConfig) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), raw, 0o600)
}

func loadConfig() (ClientConfig, error) {
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return ClientConfig{}, fmt.Errorf("load config: %w", err)
	}
	var cfg ClientConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ClientConfig{}, err
	}
	return cfg, nil
}

func saveRuntimeState(state RuntimeState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), raw, 0o600)
}

func loadRuntimeState() (RuntimeState, error) {
	raw, err := os.ReadFile(statePath())
	if err != nil {
		return RuntimeState{}, fmt.Errorf("load runtime state: %w", err)
	}
	var s RuntimeState
	if err := json.Unmarshal(raw, &s); err != nil {
		return RuntimeState{}, err
	}
	return s, nil
}

func postJSON(url string, payload any) ([]byte, error) {
	rawBody, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("request failed (%d): %s", resp.StatusCode, string(raw))
	}

	return raw, nil
}

func cmdVersion() {
	fmt.Printf("masque-linux-client %s\n", version)
	fmt.Printf("commit: %s\n", commit)
	fmt.Printf("built:  %s\n", date)
	fmt.Printf("go:     %s\n", runtime.Version())
}

func usage() {
	fmt.Println("usage: client <activate|connect|status|disconnect|doctor|version|config <path|show|export|import>> [flags]")
	fmt.Println("       client doctor -h   # list doctor flags (-control-plane, -masque-server, -strict)")
	fmt.Println("       client status -h   # list status flags (-live, -json)")
	fmt.Println("       client connect -h  # list connect flags (-check)")
	fmt.Println("       client activate -h # list activate flags (-verify)")
}

func usageConfig() {
	fmt.Println("usage: client config path")
	fmt.Println("       client config show [-json]")
	fmt.Println("       client config export [-o file] [-plain] [-force]")
	fmt.Println("       client config import -i file|- [-force] [-verify]")
}

func cmdConfigPath() {
	fmt.Println(configPath())
}

func cmdConfigShow(args []string) {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "print redacted config as JSON")
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: run client activate to create %s\n", configPath())
		os.Exit(1)
	}

	redacted := cfg
	redacted.DeviceToken = redactDeviceToken(cfg.DeviceToken)

	if *jsonOut {
		raw, err := json.MarshalIndent(redacted, "", "  ")
		if err != nil {
			fatal(err)
		}
		fmt.Println(string(raw))
		return
	}

	fmt.Println("config_file:", configPath())
	fmt.Println("control_plane_url:", redacted.ControlPlaneURL)
	fmt.Println("masque_server_url:", redacted.MasqueServerURL)
	fmt.Println("device_id:", redacted.DeviceID)
	fmt.Println("fingerprint:", redacted.Fingerprint)
	fmt.Println("device_token:", redacted.DeviceToken)
	if len(cfg.Routes) > 0 {
		fmt.Println("routes:", strings.Join(cfg.Routes, ", "))
	}
	if len(cfg.DNS) > 0 {
		fmt.Println("dns:", strings.Join(cfg.DNS, ", "))
	}
}

func redactDeviceToken(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if len(t) <= 12 {
		return "***"
	}
	return t[:6] + "…" + t[len(t)-4:]
}

func cmdConfigExport(args []string) {
	fs := flag.NewFlagSet("config export", flag.ExitOnError)
	outPath := fs.String("o", "", "write JSON to this file instead of stdout")
	plain := fs.Bool("plain", false, "include full device_token (requires -force)")
	force := fs.Bool("force", false, "acknowledge plain export risk")
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: run client activate to create %s\n", configPath())
		os.Exit(1)
	}

	outCfg := cfg
	if !*plain {
		outCfg.DeviceToken = redactDeviceToken(cfg.DeviceToken)
	} else if !*force {
		fatal(errors.New("refusing plain export without -force (writes secrets to disk if -o is set)"))
	} else {
		fmt.Fprintln(os.Stderr, "warning: plain export includes full device_token")
	}

	raw, err := json.MarshalIndent(outCfg, "", "  ")
	if err != nil {
		fatal(err)
	}

	if strings.TrimSpace(*outPath) == "" {
		fmt.Println(string(raw))
		return
	}
	if err := os.WriteFile(*outPath, raw, 0o600); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", len(raw), *outPath)
}

func cmdConfigImport(args []string) {
	fs := flag.NewFlagSet("config import", flag.ExitOnError)
	inPath := fs.String("i", "", "input JSON file, or \"-\" for stdin")
	force := fs.Bool("force", false, "overwrite existing config file")
	verify := fs.Bool("verify", false, "before save, probe control plane (/api/v1/health, /up) and masque_server /healthz")
	_ = fs.Parse(args)

	if strings.TrimSpace(*inPath) == "" {
		fatal(errors.New("-i is required (path to JSON or \"-\" for stdin)"))
	}

	var raw []byte
	var err error
	if *inPath == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(*inPath)
	}
	if err != nil {
		fatal(err)
	}

	var cfg ClientConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fatal(fmt.Errorf("parse config json: %w", err))
	}
	normalizeClientConfig(&cfg)
	if err := validateClientConfig(&cfg); err != nil {
		fatal(err)
	}

	if *verify {
		ctx := context.Background()
		client := &http.Client{Timeout: 8 * time.Second}
		cpBase := strings.TrimRight(strings.TrimSpace(cfg.ControlPlaneURL), "/")
		okURL, err := checkControlPlaneReachable(ctx, client, cpBase)
		if err != nil {
			fatal(fmt.Errorf("control plane verify failed: %w", err))
		}
		fmt.Fprintf(os.Stderr, "verify: control plane OK (%s)\n", okURL)

		msBase := strings.TrimRight(strings.TrimSpace(cfg.MasqueServerURL), "/")
		if msBase != "" {
			mHealth := joinURL(msBase, "/healthz")
			code, err := httpGetStatus(ctx, client, mHealth)
			if err != nil {
				fatal(fmt.Errorf("masque_server verify failed: %w", err))
			}
			if code != http.StatusOK {
				fatal(fmt.Errorf("masque_server verify failed: GET %s -> %d", mHealth, code))
			}
			fmt.Fprintf(os.Stderr, "verify: masque_server OK (%s)\n", mHealth)
		}
	}

	dest := configPath()
	if _, err := os.Stat(dest); err == nil && !*force {
		fatal(fmt.Errorf("refusing to overwrite %s (use -force)", dest))
	}

	if err := saveConfig(cfg); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "imported config to %s\n", dest)
}

func normalizeClientConfig(cfg *ClientConfig) {
	if cfg.Routes == nil {
		cfg.Routes = []string{}
	}
	if cfg.DNS == nil {
		cfg.DNS = []string{}
	}
}

func validateClientConfig(cfg *ClientConfig) error {
	if strings.TrimSpace(cfg.ControlPlaneURL) == "" {
		return errors.New("control_plane_url is required")
	}
	if strings.TrimSpace(cfg.MasqueServerURL) == "" {
		return errors.New("masque_server_url is required")
	}
	if strings.TrimSpace(cfg.Fingerprint) == "" {
		return errors.New("fingerprint is required")
	}
	if strings.TrimSpace(cfg.DeviceToken) == "" {
		return errors.New("device_token is required")
	}
	if cfg.DeviceID <= 0 {
		return errors.New("device_id must be positive")
	}
	return nil
}

func applyRoutes(routes []string, state *RuntimeState) error {
	for _, route := range routes {
		if strings.TrimSpace(route) == "" {
			continue
		}
		if err := run("ip", "route", "replace", route, "dev", "lo"); err != nil {
			return fmt.Errorf("apply route %s: %w", route, err)
		}
		state.AddedRoutes = append(state.AddedRoutes, route)
	}
	return nil
}

func applyDNS(servers []string, state *RuntimeState) error {
	if len(servers) == 0 {
		return nil
	}
	backup, _ := os.ReadFile("/etc/resolv.conf")
	state.ResolvConfBackup = string(backup)

	var builder strings.Builder
	for _, s := range servers {
		builder.WriteString("nameserver ")
		builder.WriteString(s)
		builder.WriteString("\n")
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write /etc/resolv.conf: %w", err)
	}
	return nil
}

func restoreRuntime(state RuntimeState) error {
	for _, route := range state.AddedRoutes {
		_ = run("ip", "route", "del", route, "dev", "lo")
	}
	if state.ResolvConfBackup != "" {
		if err := os.WriteFile("/etc/resolv.conf", []byte(state.ResolvConfBackup), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %s", name, args, strings.TrimSpace(string(out)))
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
