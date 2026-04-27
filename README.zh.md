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
- **Phase 2b（进行中）**：端到端 **MASQUE/QUIC 用户 IP 隧道**、设备 mTLS、控制面↔masque 双向 TLS 等仍在演进。当前仓库内已实现 **CONNECT-IP 协议桩**（capsule + HTTP Datagram、默认 echo ACL），并可选用 **`CONNECT_IP_UDP_RELAY`** / **`CONNECT_IP_ICMP_RELAY`** 分别做 **IPv4/UDP** 与 **IPv4 ICMP Echo（ping）** 用户态中继；**仍非**完整内核级 VPN 或全协议转发。

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

### Prometheus 指标（CONNECT-IP 相关节选）

- `masque_connect_ip_requests_total{result=...}`（含 `forbidden` 等）
- `masque_connect_ip_datagrams_received_total` / `masque_connect_ip_datagrams_sent_total`
- `masque_connect_ip_datagrams_dropped_total`、`masque_connect_ip_datagram_acl_denied_total`
- `masque_connect_ip_datagram_unknown_context_total`
- `masque_connect_ip_streams_active`
- 启用 UDP 中继时：`masque_connect_ip_udp_relay_replies_total`、`masque_connect_ip_udp_relay_errors_total{reason=...}`
- 启用 ICMP 中继时：`masque_connect_ip_icmp_relay_replies_total`、`masque_connect_ip_icmp_relay_errors_total{reason=...}`

详细英文列表见 [README.md § Current scope](./README.md#current-scope)。

## Linux 客户端（linux-client）

### 依赖能力中的 UDP 地址

`GET /v1/masque/capabilities` 中 **`transport.http3_stub.listen_udp`** 若为 `:端口`，客户端会用配置里的 **`masque_server_url` 的主机名** 拼出 QUIC 的 `host:port`。

### doctor 探测

```bash
go run ./cmd/client doctor -connect-ip
# 可选：覆盖 UDP 地址；并发送 RFC 9484 Context 0 + IPv4/UDP 到 192.0.2.1:53（需 ACL 放行）
go run ./cmd/client doctor -connect-ip -connect-ip-udp 127.0.0.1:8444 -connect-ip-rfc9484-udp
```

### connect-ip-tun（仅 Linux）

建立 **CONNECT-IP**，创建 **TUN**，在 TUN 与 **RFC 9484 Context 0** 的 HTTP Datagram 之间转发 IP 帧。需要 **`/dev/net/tun`** 及对 **`ip link` / `ip addr`** 的权限（通常 root 或 **CAP_NET_ADMIN**）。

```bash
sudo go run ./cmd/client connect-ip-tun -h
sudo go run ./cmd/client connect-ip-tun [-masque-server URL] [-connect-ip-udp host:port] \
  [-tun-name NAME] [-addr 198.18.0.1/32] [-no-address-capsule] \
  [-apply-routes-from-capsule] [-mtu 1280]
```

- **未指定 `-addr`** 时，客户端会向流上发送 **RFC 9484 ADDRESS_REQUEST**（IPv4 未指定地址），并读取服务端 **ADDRESS_ASSIGN** 胶囊后执行 **`ip addr add <分配>/前缀 dev <tun>`**（stub 常见为 **192.0.2.1/32** 等文档地址，以策略为准）。若需完全手动配置地址，使用 **`-addr`** 或 **`-no-address-capsule`**。
- 启用 **`-apply-routes-from-capsule`** 后，会尝试解析 **ROUTE_ADVERTISEMENT** 并执行 `ip route replace`（当前仅处理可精确表示为单个 CIDR 的 IPv4 范围；复杂区间会跳过并记录日志）。
- 默认服务端仍为 **echo 桩**；若开启 **`CONNECT_IP_UDP_RELAY`** / **`CONNECT_IP_ICMP_RELAY`**，则 **IPv4 UDP**（如 DNS）或 **ping** 可走真实应答，便于联调。
- 非 Linux 平台编译出的二进制执行该子命令会提示仅支持 Linux。

## 本地联调顺序（简版）

1. 启动控制面：`cd control-plane`，迁移、配置 **`MASQUE_SERVER_URL`** 指向 masque 基址（勿与 Laravel `APP_URL` 混用），`php artisan serve`。
2. 启动 masque：`cd masque-server`，设置 `CONTROL_PLANE_URL`，按需 `QUIC_LISTEN_ADDR` 等，运行 `go run ./cmd/server`。
3. 客户端：`cd linux-client`，`activate`、`doctor`、`connect` / **`connect-ip-tun`** 等。

更完整的步骤、E2E、可观测与部署说明仍以 **[README.md](./README.md)** 为准。

## 安全提示

- **`CONNECT_IP_SKIP_AUTH`**、**`CONNECT_IP_STUB_ECHO_CONTEXTS`**、**`CONNECT_IP_UDP_RELAY`**、**`CONNECT_IP_ICMP_RELAY`** 均可能扩大攻击面，仅应在受控环境使用。
- 生产环境应使用正式 TLS、强制设备鉴权，并对中继与路由做独立安全评审。
