# CONNECT-IP host TUN bridge (Linux) — operator notes

masque-server can set **`CONNECT_IP_TUN_FORWARD=1`** so each CONNECT-IP session opens a **host TUN** and bridges **RFC 9484 Context ID 0** IP datagrams to that interface after ACL (and after optional UDP/ICMP relay). The process does **not** enable **`net.ipv4.ip_forward`** or add **`iptables` NAT**.

Optional **`CONNECT_IP_TUN_LINK_UP=1`** (with **`CONNECT_IP_TUN_FORWARD`**): after each successful TUN open, masque-server runs **`ip link set dev <ifname> up`** (best-effort; requires **`ip`** on **`PATH`** and usually **`CAP_NET_ADMIN`**). This does not replace full routing/SNAT setup below.

Optional **`CONNECT_IP_TUN_MANAGED_NAT=1`** (with **`CONNECT_IP_TUN_FORWARD`**): masque-server applies minimal host networking automation on each session TUN:
- `sysctl -w net.ipv4.ip_forward=1`
- optional `ip addr replace <CONNECT_IP_TUN_ADDR_CIDR> dev <tun>`
- `iptables` FORWARD accept rules between `<tun>` and **`CONNECT_IP_TUN_EGRESS_IFACE`**
- `iptables -t nat POSTROUTING -o <egress> -j MASQUERADE`

This is a minimal bootstrap, not a full firewall policy manager. Existing host security controls remain your responsibility.

Optional **`CONNECT_IP_TUN_SHARED=1`** (with **`CONNECT_IP_TUN_FORWARD`**): all CONNECT-IP streams share one host TUN, and packets read from TUN are routed back to streams by destination IP mapping learned from inbound source IPs. This reduces per-session interface churn.
Optional **`CONNECT_IP_TUN_SHARED_BINDING_TTL`** (default `5m`) controls stale source-IP binding eviction in shared mode.

## Preconditions

- Linux host, masque-server run with access to **`/dev/net/tun`** (typically **root** or **`CAP_NET_ADMIN`**).
- Optional **`CONNECT_IP_TUN_NAME`**: interface name for **`TUNSETIFF`**; empty lets the kernel assign (e.g. `tun0`).

## Minimal host setup (example only)

Replace placeholders: **`<tun>`** (interface from logs or `CONNECT_IP_TUN_NAME`), **`<wan>`** (outbound interface toward the Internet), **`<client-pfx>`** (IPv4 prefix used on the client/TUN side — must match your addressing plan).

If **`CONNECT_IP_TUN_LINK_UP`** is **not** set, bring the interface up manually (or enable that env and run masque with sufficient privileges):

```bash
# Bring TUN up (repeat per session if names differ, or use a fixed CONNECT_IP_TUN_NAME and one long-lived session)
sudo ip link set dev <tun> up

# Allow forwarding (persistent: sysctl.d)
sudo sysctl -w net.ipv4.ip_forward=1

# SNAT traffic leaving toward WAN (broad; tighten for production)
sudo iptables -t nat -A POSTROUTING -o <wan> -j MASQUERADE

# Policy routing from TUN toward WAN is site-specific; add routes that send <client-pfx> via <tun> as needed.
```

**Security:** opening TUN + `MASQUERADE` exposes forwarding risk. Restrict by firewall, bind masque to trusted networks, and keep **device ACL** strict.

## Metrics

- **`masque_connect_ip_tun_bridge_active`**: gauge — streams currently in the TUN bridge loop.
- **`masque_connect_ip_tun_open_echo_fallback_total`**: counter — **`CONNECT_IP_TUN_FORWARD`** was on but **`openConnectIPTunForward`** failed (permission, missing `/dev/net/tun`, etc.); the stream used **echo** instead.
- **`masque_connect_ip_tun_link_up_failures_total`**: counter — **`CONNECT_IP_TUN_LINK_UP`** attempted `ip link set up` but failed (`ip` missing in `PATH`, insufficient capabilities, invalid interface state/name, etc.).
- **`masque_connect_ip_tun_managed_nat_apply_total{result}`**: countervec — managed NAT apply outcomes (`ok` or `error`).
- **`masque_connect_ip_tun_shared_binding_conflicts_total`**: counter — shared-mode source-IP ownership changes across sessions.
- **`masque_connect_ip_tun_shared_binding_stale_evictions_total`**: counter — stale source-IP bindings evicted by TTL GC.

Grafana **`ops/observability/grafana/dashboards/masque-overview.json`** includes panels for both series.

## Alerts

Prometheus rules in `ops/observability/prometheus/alerts.yml`:
- **`MasqueConnectIPTunOpenEchoFallback`**: fallback counter rate above zero for 10 minutes.
- **`MasqueConnectIPTunLinkUpFailures`**: link-up failure counter rate above zero for 10 minutes.
- **`MasqueConnectIPTunManagedNATApplyErrors`**: managed NAT apply error rate above zero for 10 minutes.
- **`MasqueConnectIPTunSharedBindingConflictsHigh`**: shared-mode binding conflict rate above `0.1/s` for 10 minutes.

Both set annotation **`runbook_url`** to this document (GitHub `blob/main` URL in-repo; override in forks if needed). **Alertmanager** forwards annotations to receivers (see `ops/observability/alertmanager/alertmanager.yml` header comment).

See also **`GET /v1/masque/capabilities`** (`quic.connect_ip.dev.tun_forward_env`) and [README.zh.md](../../README.zh.md) (CONNECT-IP metrics list). **Linux client:** `doctor` checks **`/metrics`** for TUN series when capabilities report **`tun_linux_per_session`**.
