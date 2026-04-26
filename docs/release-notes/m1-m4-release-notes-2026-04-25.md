# AFBuyers MASQUE VPN Release Notes (M1-M4)

发布日期：2026-04-25  
版本阶段：Phase 1（M1-M4）完成

## 概览

本次发布完成了从最小闭环到生产准备的四个里程碑，交付了可运行的控制面、服务端与 Linux 客户端能力，并补齐了监控告警、部署回滚、安全加固与审计完整性能力。

## 关键能力

### 1) 控制面（Laravel）

- 用户注册、登录、仪表盘、Admin 管理界面
- 用户/设备维度策略下发（ACL、路由、DNS）
- 高危操作保护：一次性确认码（5 分钟有效、单次使用）
- 会话治理：
  - 管理员空闲会话超时失效
  - 指定用户强制下线
  - 按范围批量强制下线（全体/仅非管理员）
- 审计中心：
  - 策略变更审计（含 before/after diff）
  - 审计筛选、分页、详情查看、CSV 导出
  - 审计日志防篡改哈希链（`prev_hash` + `entry_hash`）及校验命令

### 2) MASQUE Server（Go）

- `/connect` 与 `/healthz` 最小可用服务
- 对控制面进行在线鉴权回调（token/JWT-like）
- 服务端 ACL 执行（CIDR/协议/端口）
- Prometheus 指标与 `/metrics` 暴露

### 3) Linux Client（Go CLI）

- `activate/connect/status/disconnect` 基础命令闭环
- 自动应用路由与 DNS，断开后自动恢复
- 支持从控制面拉取策略（ACL、路由、DNS）

## 可观测与运维

- 观测栈：Prometheus + Grafana + Loki + Alertmanager（Docker Compose）
- 告警规则：连接失败率、延迟 P95
- 告警联通：Webhook + 本地 mock receiver + 测试脚本
- 环境验证：
  - `scripts/staging/smoke-test.sh`
  - `scripts/staging/full-check.sh`
- 部署与回滚脚本：
  - `scripts/deploy/deploy.sh`
  - `scripts/deploy/rollback.sh`

## 安全与合规增强

- HTTP 安全头与 CSP 收敛（含实际兼容修复）
- Web/API 关键写操作限流
- 生产安全开关：
  - `ALLOW_FIRST_USER_ADMIN=false`
  - `APP_ENV=production`
  - `APP_DEBUG=false`
- 审计链校验命令：
  - `php artisan audit:backfill-chain`
  - `php artisan audit:verify-chain`

## 兼容性与已知限制

- 当前为 Phase 1 交付，暂未覆盖：
  - 真正的 MASQUE 数据面隧道细节
  - 完整 mTLS 证书签发/轮换/吊销闭环
  - 完整 RBAC（当前以 `is_admin` 为基础）
  - Windows/macOS 客户端
- `full-check` 默认不跑压测（需显式设置 `RUN_K6=1`）

## 升级与操作建议

1. 先执行数据库迁移：`php artisan migrate --force`  
2. 执行一次审计链回填：`php artisan audit:backfill-chain`  
3. 校验审计链完整性：`php artisan audit:verify-chain`  
4. 运行环境检查：`./scripts/staging/full-check.sh`  
5. 生产前完成清单核对：`docs/runbooks/m4-production-readiness-checklist.md`

## 参考文档

- 生产部署手册：`docs/runbooks/production-deploy-manual.md`
- M4 验收报告：`docs/runbooks/m4-go-live-acceptance-report-2026-04-25.md`
- 项目总览：`README.md`
