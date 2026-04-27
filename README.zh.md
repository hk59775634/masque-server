# MASQUE VPN 单仓库说明（中文）

> 英文主文档见 [README.md](./README.md)。本文侧重 **CONNECT-IP / QUIC 桩、Linux 客户端 TUN、环境变量与指标**，并与英文 README 的里程碑描述对齐。

## 仓库组成

| 目录 | 说明 |
|------|------|
| `control-plane/` | Laravel 控制面（用户、设备、策略、鉴权等） |
| `masque-server/` | Go 网关：调用控制面授权、`POST /connect` ACL、可选 QUIC **HTTP/3 CONNECT-IP 桩** |
| `linux-client/` | Go CLI：`activate` / `connect` / `doctor` / **`connect-ip-tun`**（Linux）等 |
| `docs/` | 发布说明、Runbook、ADR 等 |

## 里程碑（与英文 README 一致）

- **Phase 1（M1–M4）**：控制面 + masque 最小桩 + Linux 客户端 + 可观测/部署材料；见 `docs/release-notes/`、`docs/runbooks/`。
- **Phase 2a**：**`POST /v1/masque/tcp-probe`**（服务端代拨 TCP）、主监听可选 **HTTPS**、能力字段 `tunnel.phase2a`。
- **Phase 2b（stub，本仓库已闭环）**：**CONNECT-IP 桩** + Linux **`connect-ip-tun`**（TUN、胶囊、分段默认路由、DNS 覆盖与退出恢复、**`-dns-resolvectl` 失败时可回退 `resolv.conf`（`-dns-resolvectl-fallback`）**、重连与运维向参数、`doctor -connect-ip`）；可选 **`CONNECT_IP_UDP_RELAY`** / **`CONNECT_IP_ICMP_RELAY`** / **`CONNECT_IP_ROUTE_ADV_CIDR`**；masque 在 **Linux** 上可选 **`CONNECT_IP_TUN_FORWARD`**（内核 TUN 转发）+ **`CONNECT_IP_TUN_SHARED`**（共享 TUN + 目的 IP 分流）+ **`CONNECT_IP_TUN_LINK_UP`**（`ip link up`）+ **`CONNECT_IP_TUN_MANAGED_NAT`**（托管 NAT 自动化，需 egress 配置）。**仍非** masque 侧全量策略管理器。
- **Phase 2b（生产级，仍待办）**：设备 **mTLS**、控制面↔masque **双向 TLS / 非 REST 硬化**、**RBAC**、服务端 **托管 NAT 拓扑与全 TCP·IPv6 内核路径**（见 `开发需求.md` §6）。

## QUIC / CONNECT-IP 桩（masque-server）

### 启用方式

在同一进程内设置 UDP 监听地址，例如：

```bash
cd masque-server
export CONTROL_PLANE_URL=http://127.0.0.1:8000
export QUIC_LISTEN_ADDR=:8444
go run ./cmd/server
```

- 与主 TCP 监听（默认 `:8443`）不同，**QUIC 使用独立 UDP 端口**（此处 `:8444`）。
- TLS 为进程内 **自签名** 证书，仅用于开发/联调。

### 协议行为（摘要）

- **扩展 CONNECT**，`:protocol connect-ip`（RFC 9484 形态）；**200** 前与 TCP 相同走控制面 **`/api/v1/server/authorize`**（需 **`Authorization: Bearer <device_token>`** + **`Device-Fingerprint`**），除非下文 **跳过鉴权**。
- **200** 响应带 **`Capsule-Protocol: ?1`**，流上解析 **RFC 9297** capsule；**RFC 9484** 的 **ROUTE_ADVERTISEMENT** / **ADDRESS_REQUEST** / **ADDRESS_ASSIGN** 等按桩逻辑处理（路由需在设备 ACL 的 `allow[].cidr` 内；**ADDRESS_REQUEST** 应答为文档地址 **192.0.2.1/32**、**2001:db8::1/128** 等，除非客户端请求且通过 ACL 的具体地址）。
- **HTTP Datagram（RFC 9297）**：在 QUIC 上协商；载荷前导 **QUIC varint Context ID**（**0** 表示后跟完整 **IP 包**）。非零 Context ID 默认丢弃，除非设置 **`CONNECT_IP_STUB_ECHO_CONTEXTS`**（仅开发）。
- 内层若解析为 **IPv4/IPv6**，使用与 **`POST /connect`** 相同的 **`allow` cidr / protocol / port** 做 ACL；通过则默认 **原样回显（echo）** 整帧 Datagram；非 IP 内层按不透明 echo。
- **不是**内核路由或完整路由器；完整 **TUN/内核转发** 在客户端侧见下节「Linux TUN」。

### 环境变量（开发常用）

| 变量 | 作用 |
|------|------|
| `QUIC_LISTEN_ADDR` | 非空则启用 UDP HTTP/3（health、capabilities、CONNECT-IP）。 |
| `CONNECT_IP_SKIP_AUTH` / `MASQUE_CONNECT_IP_SKIP_AUTH` | 为真时 CONNECT-IP **不要求** Bearer / Fingerprint（**禁止用于生产**）。 |
| `CONNECT_IP_STUB_ECHO_CONTEXTS` | 逗号分隔的非零 Context ID，允许剥离并进入与 IP 相同的 ACL/echo 路径（**开发用**）。 |
| **`CONNECT_IP_UDP_RELAY`** | 为真且 ACL 允许时，对**未分片 IPv4/UDP**（Context ID 0 内 IP）做**用户态中继**：向目的 IP:UDP 口转发负载，将应答封装成新的 Context 0 包发回（**无 TCP/IPv6 中继**）。能力 JSON 中 `http3_datagrams.udp_ipv4_relay`、`ip_forward` 等会反映此项。 |
| **`CONNECT_IP_ICMP_RELAY`** | 为真且 ACL 允许 **icmp** 时，对 **IPv4 ICMP Echo Request（type 8）** 代发并回送 **Echo Reply**（通常需 **root** 或 **CAP_NET_RAW** 以打开 `ip4:icmp`）。能力 JSON 含 `http3_datagrams.icmp_ipv4_echo_relay`。 |
| **`CONNECT_IP_ROUTE_ADV_CIDR`** | 设为 **IPv4 CIDR**（如 `198.18.0.0/15`）时，CONNECT-IP 在 **200 且流劫持后** 主动下发一条 **ROUTE_ADVERTISEMENT**（与入站路由相同的 ACL：整段须在某一 `allow.cidr` 内）；便于客户端 **`connect-ip-tun -apply-routes-from-capsule`** 无需先发路由 capsule。能力 JSON 在 `connect_ip.route_push` 中展示当前 CIDR。 |

### Prometheus 指标（CONNECT-IP 相关节选）

- `masque_connect_ip_requests_total{result=...}`（含 `forbidden` 等）
- `masque_connect_ip_datagrams_received_total` / `masque_connect_ip_datagrams_sent_total`
- `masque_connect_ip_datagrams_dropped_total`、`masque_connect_ip_datagram_acl_denied_total`
- `masque_connect_ip_datagram_unknown_context_total`
- `masque_connect_ip_streams_active`
- 启用 **`CONNECT_IP_TUN_FORWARD`**（Linux）时：`masque_connect_ip_tun_bridge_active`（gauge，当前处于 TUN 桥接的流数）、`masque_connect_ip_tun_open_echo_fallback_total`（TUN 打开失败回退 echo 的次数）、`masque_connect_ip_tun_link_up_failures_total`（`CONNECT_IP_TUN_LINK_UP` 执行失败次数）、`masque_connect_ip_tun_managed_nat_apply_total{result}`（`CONNECT_IP_TUN_MANAGED_NAT` 应用结果）、`masque_connect_ip_tun_shared_binding_conflicts_total` / `masque_connect_ip_tun_shared_binding_stale_evictions_total`（共享 TUN 的映射冲突/过期回收）；Grafana 见 **`ops/observability/grafana/dashboards/masque-overview.json`** 面板；Prometheus 告警 **`MasqueConnectIPTunOpenEchoFallback`** / **`MasqueConnectIPTunLinkUpFailures`** / **`MasqueConnectIPTunManagedNATApplyErrors`** / **`MasqueConnectIPTunSharedBindingConflictsHigh`**（`ops/observability/prometheus/alerts.yml`）；运维说明见 **`docs/runbooks/connect-ip-tun-forward-linux.md`**
- 配置 `CONNECT_IP_ROUTE_ADV_CIDR` 时：`masque_connect_ip_route_push_total{result=sent|invalid_cidr|acl_denied|encode_error|write_error}`
- 启用 UDP 中继时：`masque_connect_ip_udp_relay_replies_total`、`masque_connect_ip_udp_relay_errors_total{reason=...}`
- 启用 ICMP 中继时：`masque_connect_ip_icmp_relay_replies_total`、`masque_connect_ip_icmp_relay_errors_total{reason=...}`

详细英文列表见 [README.md § Current scope](./README.md#current-scope)。

## Linux 客户端（linux-client）

### 一键脚本（邮箱 + 密码）

仓库根目录 **`scripts/masque-quick-connect.sh`**：交互输入 **控制面账号邮箱与密码**，自动调用 **`POST /api/v1/devices/activation-code-with-credentials`**（携带本地持久化的 **`~/.config/masque-linux-client/device-fingerprint`**），再执行 **`masque-client activate -verify`** 与 **`connect`**。

- 默认 **`CONTROL_PLANE_URL=https://www.afbuyers.com`**，默认 **`CONNECT_MODE=dry-run`**（不改路由；真连接：`CONNECT_MODE=real ./scripts/masque-quick-connect.sh`，会 **`sudo`**）。
- 若曾用错误的 **`MASQUE_SERVER_URL`** 激活，本地 **`~/.masque-client.json`** 里可能是 **`http://127.0.0.1:8443`**：脚本在控制面非 localhost 时会自动改写成 **`DEFAULT_PUBLIC_MASQUE`**（默认 `http://www.afbuyers.com:8443`），也可设置 **`MASQUE_SERVER_URL`**；或手动：`masque-client connect -masque-server http://www.afbuyers.com:8443 ...`。
- **`connect` 与路由**：`POST /connect` 返回的 **`0.0.0.0/1`、`128.0.0.0/1`** 等策略路由**不能**装到 **`lo`**（旧版客户端曾错误地 `dev lo`，会导致 `ip route` 里出现经 loopback 的默认半表）。请使用 **`connect-ip-tun -route split`** 走 TUN，或在有自建隧道接口时 **`connect -route-dev <ifname>`**（禁止 `-route-dev lo`）。
- 已存在 **`~/.masque-client.json`** 且含 `device_token` 时，**跳过登录/激活**，直接 `connect`。
- 需已安装 **`masque-client`**（`PATH` 或设置 **`MASQUE_CLIENT=/path/to/masque-client`**）。

### 依赖能力中的 UDP 地址

`GET /v1/masque/capabilities` 中 **`transport.http3_stub.listen_udp`** 若为 `:端口`，客户端会用配置里的 **`masque_server_url` 的主机名** 拼出 QUIC 的 `host:port`。

### doctor 探测

```bash
go run ./cmd/client doctor -connect-ip
# 可选：覆盖 UDP 地址；并发送 RFC 9484 Context 0 + IPv4/UDP 到 192.0.2.1:53（需 ACL 放行）
go run ./cmd/client doctor -connect-ip -connect-ip-udp 127.0.0.1:8444 -connect-ip-rfc9484-udp
```

当 **`GET /v1/masque/capabilities`** 声明 **`tunnel.quic.connect_ip.http3_datagrams.tun_linux_per_session`**（即服务端开启 **`CONNECT_IP_TUN_FORWARD`**）时，`doctor` 会额外 **`GET /metrics`**，校验是否存在 **`masque_connect_ip_tun_bridge_active`** 与 **`masque_connect_ip_tun_open_echo_fallback_total`** 系列名；缺失则 **WARN**（常见于 masque 版本过旧或未暴露 `/metrics`）。

### connect-ip-tun（仅 Linux）

建立 **CONNECT-IP**，创建 **TUN**，在 TUN 与 **RFC 9484 Context 0** 的 HTTP Datagram 之间转发 IP 帧。需要 **`/dev/net/tun`** 及对 **`ip link` / `ip addr`** 的权限（通常 root 或 **CAP_NET_ADMIN**）。

```bash
sudo go run ./cmd/client connect-ip-tun -h
sudo go run ./cmd/client connect-ip-tun [-masque-server URL] [-connect-ip-udp host:port] \
  [-tun-name NAME] [-addr 198.18.0.1/32] [-no-address-capsule] \
  [-apply-routes-from-capsule] [-route split|all] [-dns 1.1.1.1,8.8.8.8] [-dns-resolvectl] [-dns-resolvectl-fallback=true] [-mtu 1280] [-reconnect=true] \
  [-reconnect-initial-backoff 1s] [-reconnect-max-backoff 15s] \
  [-reconnect-max-dial-failures 0] [-reconnect-max-session-drops 0] [-reconnect-log-interval 30s]
```

- **未指定 `-addr`** 时，客户端会向流上发送 **RFC 9484 ADDRESS_REQUEST**（IPv4 未指定地址），并读取服务端 **ADDRESS_ASSIGN** 胶囊后执行 **`ip addr add <分配>/前缀 dev <tun>`**（stub 常见为 **192.0.2.1/32** 等文档地址，以策略为准）。若需完全手动配置地址，使用 **`-addr`** 或 **`-no-address-capsule`**。
- 启用 **`-apply-routes-from-capsule`** 后，会尝试解析 **ROUTE_ADVERTISEMENT** 并执行 `ip route replace`（当前仅处理可精确表示为单个 CIDR 的 IPv4 范围；复杂区间会跳过并记录日志）。
- 默认 **`-reconnect=true`**：若 QUIC/CONNECT-IP 会话中断，会保留同一个 TUN 接口并按退避重连（`-reconnect-initial-backoff` 到 `-reconnect-max-backoff`），减少手工重启客户端。
- **`-reconnect-max-dial-failures N`**：`N>0` 时连续 **拨号** 失败达到 `N` 次则退出（成功拨号后计数清零；`0` 表示不限制）。
- **`-reconnect-log-interval`**：拨号失败、会话结束类日志的最小间隔（默认 `30s`，`0` 表示每次都打日志），长时间断网时减轻刷屏。
- **`-reconnect-max-session-drops N`**：`N>0` 时，在**已成功拨号**之后，若连续 **N** 次会话异常结束则退出（每次成功拨号后计数清零；`0` 不限制）。
- **`-route split`** 或 **`-route all`**（与 **`-split-default-route`** 二选一即可，`all` 为 `split` 别名）：安装 **IPv4 分段默认路由** `0.0.0.0/1` 与 `128.0.0.0/1` 经 TUN（对齐 `开发需求.md` §7.3）；**`-route none`** 显式关闭，且会覆盖 `-split-default-route`；退出时自动删除。
- **`-split-default-route`**：与 **`-route split`** 等价（保留兼容）；更推荐 **`-route`**。
- **`-dns a,b,c`**：覆盖 **`/etc/resolv.conf`**（先读备份，进程退出时恢复）；**需要 root**；若检测到 **`systemd-resolved` stub** 链到该文件，会打一条 **warn**。
- **`-dns-resolvectl`**（与 **`-dns`** 联用）：走 **`resolvectl dns <默认路由网卡> …`**，退出时 **`resolvectl revert`**（需 **`resolvectl`** 与 **systemd-resolved**；通常仍需 **root**）。适合桌面环境避免直接改 stub **`resolv.conf`**。
- **`-dns-resolvectl-fallback`**（默认 **`true`**）：**`-dns-resolvectl`** 失败时自动改用 **`/etc/resolv.conf`**（与不加 **`-dns-resolvectl`** 相同）；设为 **`false`** 则 **`resolvectl`** 失败直接退出（强制只用 systemd-resolved 路径）。
- **`-bypass-masque-host`**（默认 `true`，且仅在启用分段默认路由时生效）：为 **QUIC 目标主机**（及 `-masque-server` 若解析出不同 IPv4）添加 **`/32` 经当前默认网关** 的绕行，避免黑洞。
- 默认服务端仍为 **echo 桩**；若开启 **`CONNECT_IP_UDP_RELAY`** / **`CONNECT_IP_ICMP_RELAY`**，则 **IPv4 UDP**（如 DNS）或 **ping** 可走真实应答，便于联调。
- **masque-server（Linux）**：**`CONNECT_IP_TUN_FORWARD=1`** 时，每个 CONNECT-IP 会话打开一个 **host TUN**，将 ACL 允许的 **Context 0 IP** 写入 TUN，并从 TUN 读回包发给客户端（**`CONNECT_IP_TUN_NAME`** 可选，对应 TUNSETIFF）。可选 **`CONNECT_IP_TUN_LINK_UP=1`**：每次 TUN 打开成功后执行 **`ip link set dev <if> up`**（需 **`PATH` 中有 `ip`**，通常 **`CAP_NET_ADMIN`**）。**`net.ipv4.ip_forward`** 与 **SNAT**（如 **`iptables -t nat MASQUERADE`**）仍由运维配置，进程内不执行。
- 非 Linux 平台编译出的二进制执行该子命令会提示仅支持 Linux。

## 本地联调顺序（简版）

1. 启动控制面：`cd control-plane`，迁移、配置 **`MASQUE_SERVER_URL`** 指向 masque 基址（勿与 Laravel `APP_URL` 混用），`php artisan serve`。
2. 启动 masque：`cd masque-server`，设置 `CONTROL_PLANE_URL`，按需 `QUIC_LISTEN_ADDR` 等，运行 `go run ./cmd/server`。
3. 客户端：`cd linux-client`，`activate`、`doctor`、`connect` / **`connect-ip-tun`** 等。

更完整的步骤、E2E、可观测与部署说明仍以 **[README.md](./README.md)** 为准。

## 安全提示

- **`CONNECT_IP_SKIP_AUTH`**、**`CONNECT_IP_STUB_ECHO_CONTEXTS`**、**`CONNECT_IP_UDP_RELAY`**、**`CONNECT_IP_ICMP_RELAY`**、**`CONNECT_IP_TUN_FORWARD`**、**`CONNECT_IP_TUN_SHARED`**、**`CONNECT_IP_TUN_SHARED_BINDING_TTL`**、**`CONNECT_IP_TUN_LINK_UP`**、**`CONNECT_IP_TUN_MANAGED_NAT`** 均可能扩大攻击面，仅应在受控环境使用。
- 生产环境应使用正式 TLS、强制设备鉴权，并对中继与路由做独立安全评审。

## 告警 Mock 接收器提示配置

- 文件：`scripts/alerts/suggestions.yml`
- 作用：为 `scripts/alerts/mock_receiver.py` 按 `alertname` 显示“建议下一步排查”。
- 语法（简化 YAML）：
  - 顶层键：告警名（如 `MasqueConnectIPTunLinkUpFailures`）
  - `steps`：`- ` 开头的排查步骤列表
  - `commands`（可选）：`- ` 开头的命令模板列表（mock receiver 会单独打印，方便复制执行）
- 发送测试告警：`scripts/alerts/send-test-alert.sh` 支持 `--alertname/--severity/--summary/--description/--runbook-url`；例如：
  - `./scripts/alerts/send-test-alert.sh --alertname MasqueConnectIPTunLinkUpFailures --severity warning`
  - `./scripts/alerts/send-test-alert.sh --dry-run --alertname MasqueConnectIPTunOpenEchoFallback`（仅预览 JSON，不发送）

## Phase 2b 内核转发验收脚本

- 脚本：`scripts/staging/phase2b-kernel-check.sh`
- 默认检查：
  - `GET /v1/masque/capabilities` 中 `tun_linux_per_session/tun_linux_managed_nat/tun_linux_shared`
  - `GET /metrics` 中共享 TUN/托管 NAT 关键指标名
  - Prometheus 已加载相关告警规则
- 集成到 full-check：`RUN_PHASE2B_KERNEL=1 ./scripts/staging/full-check.sh`
- GitHub Actions 手动门禁：
  - 触发 `CI` 的 `workflow_dispatch`
  - 设置 `run_phase2b_kernel=true`，并填写 staging 的 `control_plane_url/masque_server_url/prometheus_url/alertmanager_url`（可选 `loki_url/grafana_url`）
  - 会执行 `phase2b-kernel-staging` 任务，内部调用 `full-check.sh` 并开启 `RUN_PHASE2B_KERNEL=1`
