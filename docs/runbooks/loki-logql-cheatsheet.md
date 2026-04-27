# Loki / LogQL 备忘（AFBuyers）

面向在 **Grafana Explore → Loki** 里做临时排查的同事；与 `ops/observability/grafana/dashboards/loki-logql-cheatsheet.json` 面板内容互补。

## 入口

- 本地 Compose：`http://127.0.0.1:3000/explore`（默认账号见 `ops/observability/docker-compose.yml`）。
- 管理后台「运营概览」中有 Grafana / Explore / **Loki 备忘面板** 快捷链接（`GRAFANA_URL` 等需在 `.env` 与生产反代对齐）。

## 常用 LogQL

| 场景 | 示例 |
|------|------|
| 按 job 选流 | `{job="masque-server"}` |
| 子串过滤 | `{job="masque-server"} \|= "error"` |
| 排除噪音 | `{job="masque-server"} \|!= "healthz"` |
| JSON 字段 | `{job="app"} \| json \| field > 100` |
| 每分钟行数 | `sum(count_over_time({job="masque-server"}[1m]))` |

## Loki 就绪

- `GET /ready`：**200** 表示就绪；刚启动时可能出现 **503**（ring 未就绪），可稍后重试。

## 安全

- 勿把 Grafana 管理账号或观测栈端口直接暴露到公网；生产请走 VPN 或内网反代。
