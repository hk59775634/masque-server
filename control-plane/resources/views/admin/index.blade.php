<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Admin 控制中心 - MASQUE</title>
    <style>
        :root { --bg:#0b1020;--bg2:#0f172a;--card:#111a2f;--line:#243557;--text:#dbe6ff;--muted:#8ea2c6;--accent:#63b3ff;--ok:#5eead4;--warn:#fda4af;}
        *{box-sizing:border-box}
        body{margin:0;background:radial-gradient(1000px 520px at -5% -10%,#1e40af 0%,transparent 55%),radial-gradient(900px 460px at 110% 10%,#0ea5e9 0%,transparent 45%),var(--bg);font-family:"Segoe UI","PingFang SC",sans-serif;color:var(--text)}
        .wrap{max-width:1320px;margin:24px auto;padding:0 16px}
        .top{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-bottom:14px}
        h1{margin:0;font-size:28px}.muted{color:var(--muted)}
        .actions,.tabs{display:flex;gap:8px;flex-wrap:wrap}.btn,.tab{display:inline-flex;align-items:center;gap:8px;padding:10px 14px;border-radius:10px;border:1px solid var(--line);text-decoration:none;color:var(--text);background:#0b1222}
        .tab.active{background:#14305f;border-color:#2f5fb5}
        .ok{margin:12px 0;padding:10px 12px;border:1px solid #22796e;background:#0f2f34;border-radius:10px;color:#91f7eb}
        .err{margin:12px 0;padding:10px 12px;border:1px solid #7f1d1d;background:#3f1118;border-radius:10px;color:#fecaca}
        .layout{display:grid;grid-template-columns:320px 1fr;gap:14px}
        .card{background:linear-gradient(180deg,#121d34 0%,#0c1527 100%);border:1px solid var(--line);border-radius:14px;padding:14px}
        .card h2{margin:0 0 10px;font-size:17px}
        .list{max-height:630px;overflow:auto;border:1px solid #22365d;border-radius:10px}
        .item{display:block;padding:10px;border-bottom:1px solid #1a2947;color:var(--text);text-decoration:none}
        .item:hover{background:#122242}.item.active{background:#183566}
        .pill{font-size:11px;padding:2px 7px;border-radius:999px;background:#173b70}
        .pill.warn{background:#4b2432;color:#fecdd3}.pill.ok{background:#114c4d;color:#99f6e4}
        .editor{display:grid;grid-template-columns:1fr 1fr;gap:14px}
        .subcard{border:1px solid #21355b;border-radius:12px;padding:12px;background:#0b1325}
        label{display:block;font-size:13px;color:#aac1e8;margin-bottom:6px}
        input,select,button,textarea{font:inherit}
        input,select,textarea{width:100%;padding:9px 10px;border-radius:9px;border:1px solid #30486f;background:#060b18;color:#dbe6ff;margin-bottom:8px}
        .aclrow{display:grid;grid-template-columns:1fr 120px 120px 40px;gap:8px;align-items:center;margin-bottom:8px}
        .aclrow button{padding:8px;border-radius:8px;border:1px solid #5b2440;background:#3a1125;color:#fecdd3;cursor:pointer}
        .smallbtn{padding:7px 10px;border-radius:8px;border:1px solid #30548a;background:#13284f;color:#c7dcff;cursor:pointer}
        .save{padding:10px 14px;border:none;border-radius:10px;background:linear-gradient(120deg,#7dd3fc,#60a5fa);color:#041428;font-weight:700;cursor:pointer}
        pre{margin:0;padding:10px;border:1px solid #2a3e65;border-radius:10px;background:#050a16;max-height:260px;overflow:auto;color:#9ac2ff;font-size:12px}
        table{width:100%;border-collapse:collapse;font-size:13px}
        th,td{border-bottom:1px solid #213558;padding:8px;text-align:left;vertical-align:top}
        th{color:#a9c0e7;font-weight:600}
        .filter-grid{display:grid;grid-template-columns:repeat(7,minmax(100px,1fr));gap:8px;margin-bottom:10px}
        .pagination{margin-top:12px}
        .pagination svg{width:14px;height:14px}
        .pagination nav div:first-child{display:none}
        .pagination span,.pagination a{color:#c8daff!important;background:#0b1325!important;border-color:#24406b!important}
        .audit-open{padding:5px 8px;border-radius:7px;border:1px solid #2d538a;background:#132a52;color:#d1e4ff;cursor:pointer}
        .modal{position:fixed;inset:0;background:rgba(2,6,23,.72);display:none;align-items:center;justify-content:center;padding:20px;z-index:50}
        .modal.show{display:flex}
        .modal-card{width:min(900px,96vw);max-height:88vh;overflow:auto;border:1px solid #2a4b82;border-radius:14px;background:#0a1327;padding:14px}
        .modal-top{display:flex;justify-content:space-between;gap:10px;align-items:center;margin-bottom:10px}
        .modal-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px}
        .kv{border:1px solid #25406a;border-radius:10px;padding:10px;background:#071124}
        .kv h4{margin:0 0 8px;color:#a6c8ff}
        .diff-added{color:#99f6e4}
        .diff-removed{color:#fca5a5}
        .diff-path{color:#93c5fd}
        .token-row{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-top:8px}
        .status-chip{display:inline-block;padding:2px 8px;border-radius:999px;font-size:12px;border:1px solid}
        .status-chip.ok{color:#99f6e4;background:#114c4d;border-color:#1b6b6c}
        .status-chip.err{color:#fecaca;background:#4b2432;border-color:#7f1d1d}
        .archive-error-link{background:transparent;border:none;color:#fca5a5;cursor:pointer;text-decoration:underline;text-align:left;font:inherit;padding:0;max-width:100%;word-break:break-word}
        .archive-error-link:hover{color:#fecaca}
        .fail-dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:#f87171;margin-right:6px;vertical-align:middle;flex-shrink:0;box-shadow:0 0 0 1px rgba(0,0,0,.25)}
        .event-cell{display:inline-flex;align-items:center;gap:2px;flex-wrap:wrap;max-width:100%}
        @media (max-width:1080px){.layout{grid-template-columns:1fr}.editor{grid-template-columns:1fr}.aclrow{grid-template-columns:1fr 90px 90px 40px}.filter-grid{grid-template-columns:1fr 1fr}}
    </style>
</head>
<body>
<div class="wrap">
    <div class="top">
        <div>
            <h1>Admin 控制中心</h1>
            <div class="muted">策略下发、ACL 控制、审计追踪（可视化编辑版）。</div>
        </div>
        <div class="actions">
            <a class="btn" href="{{ route('dashboard') }}">用户首页</a>
            <a class="btn" href="{{ route('admin.index', ['tab' => 'users']) }}">用户策略</a>
            <a class="btn" href="{{ route('admin.index', ['tab' => 'devices']) }}">设备策略</a>
            <a class="btn" href="{{ route('admin.audits', ['tab' => 'audits']) }}">操作审计</a>
            @if(auth()->user()?->hasPermission('admin.rbac.write'))
            <a class="btn" href="{{ route('admin.rbac.index') }}">角色与权限</a>
            @endif
            <a class="btn" href="{{ route('admin.two-factor.setup') }}">两步验证</a>
            <a class="btn" href="{{ url('/docs/api') }}" target="_blank" rel="noopener noreferrer">API 文档</a>
            <form method="POST" action="{{ route('logout') }}">@csrf <button class="btn" type="submit">退出登录</button></form>
        </div>
    </div>

    @if(session('status'))
        <div class="ok">{{ session('status') }}</div>
    @endif
    @if($errors->any())
        <div class="err">
            @foreach($errors->all() as $error)
                <div>{{ $error }}</div>
            @endforeach
        </div>
    @endif

    @php
        $selectedUser = $users->firstWhere('id', $selectedUserId) ?? $users->first();
        $selectedDevice = $devices->firstWhere('id', $selectedDeviceId) ?? $devices->first();
        $tab = in_array($tab, ['overview', 'users', 'devices', 'audits'], true) ? $tab : 'users';
    @endphp

    <div class="tabs" style="margin-bottom:12px;">
        <a class="tab {{ $tab === 'overview' ? 'active' : '' }}" href="{{ route('admin.index', ['tab' => 'overview']) }}">运营概览</a>
        <a class="tab {{ $tab === 'users' ? 'active' : '' }}" href="{{ route('admin.index', ['tab' => 'users', 'user_id' => $selectedUser?->id]) }}">用户策略</a>
        <a class="tab {{ $tab === 'devices' ? 'active' : '' }}" href="{{ route('admin.index', ['tab' => 'devices', 'device_id' => $selectedDevice?->id]) }}">设备策略与 ACL</a>
        <a class="tab {{ $tab === 'audits' ? 'active' : '' }}" href="{{ route('admin.audits', ['tab' => 'audits']) }}">操作审计</a>
    </div>
    @if($tab === 'overview')
        <div class="card" style="margin-bottom:12px;">
            <h2>控制面与设备</h2>
            <div class="filter-grid" style="grid-template-columns: repeat(4, minmax(120px, 1fr));">
                <div class="subcard"><strong>用户总数</strong><div>{{ $opsOverview['db']['users_total'] ?? 0 }}</div></div>
                <div class="subcard"><strong>设备总数</strong><div>{{ $opsOverview['db']['devices_total'] ?? 0 }}</div></div>
                <div class="subcard"><strong>24h 内活跃设备</strong><div>{{ $opsOverview['db']['devices_seen_24h'] ?? 0 }}</div></div>
                <div class="subcard"><strong>7 日内活跃设备</strong><div>{{ $opsOverview['db']['devices_seen_7d'] ?? 0 }}</div></div>
            </div>
        </div>
        <div class="card" style="margin-bottom:12px;">
            <h2>MASQUE 服务端（Prometheus）</h2>
            @php
                $prom = $opsOverview['prometheus'] ?? ['reachable' => false, 'error' => '无数据', 'metrics' => []];
                $m = $prom['metrics'] ?? [];
                $req = $m['connect_requests'] ?? null;
                $succ = $m['connect_success'] ?? null;
                $fail = $m['connect_failures_sum'] ?? null;
                $hz = $m['healthz_requests'] ?? null;
                $up = $m['target_up'] ?? null;
                $rate = ($req !== null && $req > 0 && $fail !== null) ? round(100 * $fail / $req, 2) : null;
            @endphp
            @if(!($prom['reachable'] ?? false))
                <div class="muted">Prometheus 不可用：{{ $prom['error'] ?? '未知原因' }}。可在 <code>.env</code> 设置 <code>PROMETHEUS_URL</code>，并确保抓取任务 <code>masque-server</code> 已上线。</div>
            @else
                <div class="filter-grid" style="grid-template-columns: repeat(3, minmax(140px, 1fr));">
                    <div class="subcard"><strong>抓取目标 up</strong><div>{{ $up === null ? '—' : (int) $up }}</div></div>
                    <div class="subcard"><strong>connect 请求累计</strong><div>{{ $req === null ? '—' : (int) $req }}</div></div>
                    <div class="subcard"><strong>connect 成功累计</strong><div>{{ $succ === null ? '—' : (int) $succ }}</div></div>
                    <div class="subcard"><strong>connect 失败累计</strong><div>{{ $fail === null ? '—' : (int) $fail }}</div></div>
                    <div class="subcard"><strong>失败占比（失败/请求）</strong><div>{{ $rate === null ? '—' : $rate.'%' }}</div></div>
                    <div class="subcard"><strong>healthz 命中累计</strong><div>{{ $hz === null ? '—' : (int) $hz }}</div></div>
                </div>
            @endif
            <p class="muted" style="margin-top:12px;margin-bottom:0;">数据面能力占位：<a href="{{ url('/docs/api') }}" target="_blank" rel="noopener noreferrer">OpenAPI</a> 与节点 <code>GET http://&lt;masque&gt;:8443/v1/masque/capabilities</code>（需直连节点）。</p>
        </div>
        <div class="card" style="margin-bottom:12px;">
            <h2>观测栈（Docker Compose）</h2>
            <p class="muted" style="margin-top:0;">默认对应仓库 <code>ops/observability/docker-compose.yml</code> 映射端口；Grafana 初始账号密码见该文件中的环境变量（勿暴露到公网）。生产环境请用下方 URL 环境变量改为内网域名或反代地址。</p>
            <div class="actions" style="margin-top:8px;">
                <a class="btn" href="{{ $opsOverview['ui']['grafana'] ?? '#' }}" target="_blank" rel="noopener noreferrer">Grafana</a>
                <a class="btn" href="{{ $opsOverview['ui']['grafana_explore'] ?: ($opsOverview['ui']['grafana'] ?? '#') }}" target="_blank" rel="noopener noreferrer">Grafana Explore</a>
                <a class="btn" href="{{ $opsOverview['ui']['prometheus'] ?? '#' }}" target="_blank" rel="noopener noreferrer">Prometheus</a>
                <a class="btn" href="{{ $opsOverview['ui']['alertmanager'] ?? '#' }}" target="_blank" rel="noopener noreferrer">Alertmanager</a>
                <a class="btn" href="{{ $opsOverview['ui']['alertmanager_silences'] ?? '#' }}" target="_blank" rel="noopener noreferrer">静默列表</a>
                <a class="btn" href="{{ $opsOverview['ui']['alertmanager_silence_new'] ?? '#' }}" target="_blank" rel="noopener noreferrer">新建静默</a>
                <a class="btn" href="{{ $opsOverview['ui']['loki'] ?? '#' }}" target="_blank" rel="noopener noreferrer">Loki</a>
                <a class="btn" href="{{ $opsOverview['ui']['loki_ready'] ?? '#' }}" target="_blank" rel="noopener noreferrer">Loki /ready</a>
                <a class="btn" href="{{ $opsOverview['ui']['grafana_loki_cheatsheet'] ?: '#' }}" target="_blank" rel="noopener noreferrer">Loki 备忘面板</a>
            </div>
        </div>
    @endif
    <div class="card" style="margin-bottom:12px;">
        <h2>高危操作一次性确认码（5分钟有效，单次使用）</h2>
        <form method="POST" action="{{ route('admin.operation-token') }}" class="actions">
            @csrf
            <input type="hidden" name="tab" value="{{ $tab }}">
            <button class="smallbtn" type="submit">生成一次性确认码</button>
        </form>
        @if($operationToken !== '')
            <div class="ok" style="margin-top:10px;" data-token-card data-token-expire="{{ $operationTokenExpiresAt }}">
                <div class="token-row">
                    当前确认码：<strong id="operation-token-text">{{ $operationToken }}</strong>
                    <button class="smallbtn" type="button" id="copy-operation-token">一键复制</button>
                </div>
                <div class="token-row">
                    <span>过期时间：{{ $operationTokenExpiresAt }}</span>
                    <span id="operation-token-countdown">剩余有效时间计算中...</span>
                </div>
            </div>
        @endif
    </div>
    @if($tab === 'users')
        <div class="card" style="margin-bottom:12px;">
            <h2>批量强制下线（高危操作）</h2>
            <form method="POST" action="{{ route('admin.users.force-logout-scope') }}" onsubmit="return confirm('确认执行批量强制下线？');">
                @csrf
                <label>作用范围</label>
                <select name="scope">
                    <option value="non_admin_users">仅非管理员用户</option>
                    <option value="all_users">全部用户（不含当前操作者）</option>
                </select>
                <label>一次性确认码</label>
                <input name="operation_token" placeholder="例如 ABC-DEF">
                <button class="smallbtn" type="submit">执行批量强制下线</button>
            </form>
        </div>
    @endif

    @if($tab === 'audits')
        <div class="card">
            <h2>操作审计页</h2>
            <div class="filter-grid" style="margin-bottom:12px;">
                <div class="subcard"><strong>未归档</strong><div>{{ $auditStats['active_count'] ?? 0 }}</div></div>
                <div class="subcard"><strong>已归档</strong><div>{{ $auditStats['archived_count'] ?? 0 }}</div></div>
                <div class="subcard" style="grid-column: span 2;"><strong>最近归档时间</strong><div>{{ $auditStats['last_archived_at'] ?? '暂无' }}</div></div>
            </div>
            <div class="subcard" style="margin-bottom:12px;">
                <strong>归档任务状态：</strong>
                {{ !empty($archiveJobRunning) ? '进行中' : '空闲' }}
            </div>
            <div class="subcard" style="margin-bottom:12px;">
                <h4 style="margin:0 0 8px;">最近一次执行结果摘要</h4>
                @if(!empty($archiveLastRun))
                    @php $lastRunSuccess = (bool) ($archiveLastRun->metadata['success'] ?? true); @endphp
                    <div>执行时间：{{ $archiveLastRun->created_at }}</div>
                    <div>
                        执行状态：
                        <span class="status-chip {{ $lastRunSuccess ? 'ok' : 'err' }}">{{ $lastRunSuccess ? '成功' : '失败' }}</span>
                    </div>
                    <div>归档条数：{{ $archiveLastRun->metadata['archived_count'] ?? '-' }}</div>
                    <div>阈值(天)：{{ $archiveLastRun->metadata['days'] ?? '-' }}</div>
                    <div>操作者：{{ $archiveLastRun->metadata['operator_email'] ?? '-' }}</div>
                    @if(!$lastRunSuccess)
                        <div>
                            失败原因：
                            <button
                                type="button"
                                class="archive-error-link"
                                data-audit-open
                                title="点击查看完整审计详情"
                                data-created="{{ $archiveLastRun->created_at }}"
                                data-event="{{ $archiveLastRun->event_type }}"
                                data-operator="{{ $archiveLastRun->metadata['operator_email'] ?? '-' }}"
                                data-target-user="{{ $archiveLastRun->user_id ?? '-' }}"
                                data-target-device="{{ $archiveLastRun->device_id ?? '-' }}"
                                data-message="{{ $archiveLastRun->message }}"
                                data-ip="{{ $archiveLastRun->ip_address ?? '-' }}"
                                data-metadata='@json($archiveLastRun->metadata ?? [])'
                            >{{ $archiveLastRun->metadata['error'] ?? '未知错误' }}</button>
                        </div>
                    @endif
                    <div class="actions" style="margin-top:8px;">
                        <a class="smallbtn" href="{{ route('admin.audits', ['tab' => 'audits', 'include_archived' => 1, 'event_type' => 'admin.audit_archive_run']) }}">查看最新归档记录</a>
                    </div>
                @else
                    <div>暂无执行记录</div>
                @endif
            </div>
            <form method="POST" action="{{ route('admin.audits.archive-now') }}" data-archive-form onsubmit="return confirm('确认立即执行审计归档？该操作会标记历史日志为已归档。');" style="margin-bottom:12px;">
                @csrf
                <div class="filter-grid">
                    <input type="number" name="days" min="30" max="3650" value="180" placeholder="归档阈值（天）">
                    <input name="operation_token" placeholder="一次性确认码（例如 ABC-DEF）">
                    <button class="smallbtn" type="submit" data-archive-submit>立即执行审计归档</button>
                </div>
            </form>
            <div class="subcard" style="margin-bottom:12px;">
                <h4 style="margin:0 0 8px;">最近归档执行记录</h4>
                <table>
                    <thead><tr><th>时间</th><th>状态</th><th>阈值(天)</th><th>归档条数</th><th>操作者</th><th>错误摘要</th><th>操作</th></tr></thead>
                    <tbody>
                    @forelse($archiveRuns as $run)
                        @php $runSuccess = (bool) ($run->metadata['success'] ?? true); @endphp
                        <tr>
                            <td>{{ $run->created_at }}</td>
                            <td><span class="status-chip {{ $runSuccess ? 'ok' : 'err' }}">{{ $runSuccess ? '成功' : '失败' }}</span></td>
                            <td>{{ $run->metadata['days'] ?? '-' }}</td>
                            <td>{{ $run->metadata['archived_count'] ?? '-' }}</td>
                            <td>{{ $run->metadata['operator_email'] ?? '-' }}</td>
                            <td>
                                @if(!$runSuccess)
                                    <button
                                        type="button"
                                        class="archive-error-link"
                                        data-audit-open
                                        title="点击查看完整审计详情"
                                        data-created="{{ $run->created_at }}"
                                        data-event="{{ $run->event_type }}"
                                        data-operator="{{ $run->metadata['operator_email'] ?? '-' }}"
                                        data-target-user="{{ $run->user_id ?? '-' }}"
                                        data-target-device="{{ $run->device_id ?? '-' }}"
                                        data-message="{{ $run->message }}"
                                        data-ip="{{ $run->ip_address ?? '-' }}"
                                        data-metadata='@json($run->metadata ?? [])'
                                    >{{ \Illuminate\Support\Str::limit((string) ($run->metadata['error'] ?? '未知错误'), 80) }}</button>
                                @else
                                    -
                                @endif
                            </td>
                            <td>
                                <button
                                    type="button"
                                    class="audit-open"
                                    data-audit-open
                                    data-created="{{ $run->created_at }}"
                                    data-event="{{ $run->event_type }}"
                                    data-operator="{{ $run->metadata['operator_email'] ?? '-' }}"
                                    data-target-user="{{ $run->user_id ?? '-' }}"
                                    data-target-device="{{ $run->device_id ?? '-' }}"
                                    data-message="{{ $run->message }}"
                                    data-ip="{{ $run->ip_address ?? '-' }}"
                                    data-metadata='@json($run->metadata ?? [])'
                                >查看详情</button>
                            </td>
                        </tr>
                    @empty
                        <tr><td colspan="7">暂无归档执行记录</td></tr>
                    @endforelse
                    </tbody>
                </table>
            </div>
            <form method="GET" action="{{ route('admin.audits') }}">
                <input type="hidden" name="tab" value="audits">
                <div class="filter-grid">
                    <div>
                        <input name="event_type" value="{{ request('event_type') }}" placeholder="事件类型（可输入）" list="event-type-options">
                        <datalist id="event-type-options">
                            @foreach($auditEventTypes as $eventType)
                                <option value="{{ $eventType }}"></option>
                            @endforeach
                        </datalist>
                    </div>
                    <input name="operator_email" value="{{ request('operator_email') }}" placeholder="操作人邮箱">
                    <input name="filter_user_id" value="{{ request('filter_user_id') }}" placeholder="目标用户ID">
                    <input name="filter_device_id" value="{{ request('filter_device_id') }}" placeholder="目标设备ID">
                    <input type="date" name="from" value="{{ request('from') }}">
                    <input type="date" name="to" value="{{ request('to') }}">
                    <label style="display:flex;align-items:center;gap:6px;margin:0;">
                        <input type="checkbox" name="include_archived" value="1" {{ !empty($includeArchived) ? 'checked' : '' }} style="width:auto;margin:0;">
                        包含已归档
                    </label>
                    <div class="actions">
                        <button class="smallbtn" type="submit">筛选</button>
                        <a class="smallbtn" href="{{ route('admin.audits', array_merge(request()->query(), ['tab' => 'audits', 'event_type' => 'admin.audit_archive_run'])) }}">仅看归档执行事件</a>
                        <a class="smallbtn" href="{{ route('admin.audits', ['tab' => 'audits']) }}">重置</a>
                        <a class="smallbtn" href="{{ route('admin.audits.export', request()->query()) }}">导出CSV</a>
                    </div>
                </div>
            </form>
            <table>
                <thead><tr><th>时间</th><th>事件</th><th>操作人</th><th>目标</th><th>内容</th><th>详情</th></tr></thead>
                <tbody>
                @foreach($audits as $log)
                    @php
                        $logMeta = $log->metadata ?? [];
                        $logErr = isset($logMeta['error']) ? trim((string) $logMeta['error']) : '';
                        $logExplicitFail = array_key_exists('success', $logMeta) && $logMeta['success'] === false;
                        $logShowErrLink = ($logErr !== '') || $logExplicitFail;
                        $logErrLabel = $logErr !== ''
                            ? \Illuminate\Support\Str::limit($logErr, 72)
                            : '失败：点击查看详情';
                        $logArchiveRunFailed = $log->event_type === 'admin.audit_archive_run'
                            && ($logExplicitFail || $logErr !== '');
                    @endphp
                    <tr>
                        <td>{{ $log->created_at }}</td>
                        <td>
                            <span class="event-cell">
                                @if($logArchiveRunFailed)
                                    <span class="fail-dot" title="归档执行失败" aria-hidden="true"></span>
                                @endif
                                <span>{{ $log->event_type }}</span>
                            </span>
                        </td>
                        <td>{{ $log->metadata['operator_email'] ?? '-' }}</td>
                        <td>user:{{ $log->user_id ?? '-' }} / device:{{ $log->device_id ?? '-' }}</td>
                        <td>
                            <div>{{ $log->message }}</div>
                            @if($logShowErrLink)
                                <div style="margin-top:6px;">
                                    <button
                                        type="button"
                                        class="archive-error-link"
                                        data-audit-open
                                        title="点击查看完整审计详情"
                                        data-created="{{ $log->created_at }}"
                                        data-event="{{ $log->event_type }}"
                                        data-operator="{{ $log->metadata['operator_email'] ?? '-' }}"
                                        data-target-user="{{ $log->user_id ?? '-' }}"
                                        data-target-device="{{ $log->device_id ?? '-' }}"
                                        data-message="{{ $log->message }}"
                                        data-ip="{{ $log->ip_address ?? '-' }}"
                                        data-metadata='@json($log->metadata ?? [])'
                                    >{{ $logErrLabel }}</button>
                                </div>
                            @endif
                        </td>
                        <td>
                            <button
                                type="button"
                                class="audit-open"
                                data-audit-open
                                data-created="{{ $log->created_at }}"
                                data-event="{{ $log->event_type }}"
                                data-operator="{{ $log->metadata['operator_email'] ?? '-' }}"
                                data-target-user="{{ $log->user_id ?? '-' }}"
                                data-target-device="{{ $log->device_id ?? '-' }}"
                                data-message="{{ $log->message }}"
                                data-ip="{{ $log->ip_address ?? '-' }}"
                                data-metadata='@json($log->metadata ?? [])'
                            >查看</button>
                        </td>
                    </tr>
                @endforeach
                </tbody>
            </table>
            <div class="pagination">{{ $audits->links() }}</div>
        </div>
    @else
    <div class="layout">
        <div class="card">
            <h2>{{ $tab === 'users' ? '用户列表' : '设备列表' }}</h2>
            <div class="list">
                @if($tab === 'users')
                    @foreach($users as $user)
                        <a class="item {{ $selectedUser && $selectedUser->id === $user->id ? 'active' : '' }}" href="{{ route('admin.index', ['tab' => 'users', 'user_id' => $user->id]) }}">
                            {{ $user->name }} <small>({{ $user->email }})</small>
                            @if($user->is_admin)<span class="pill ok">admin</span>@endif
                            <br><small>ID {{ $user->id }}</small>
                        </a>
                    @endforeach
                @else
                    @foreach($devices as $device)
                        <a class="item {{ $selectedDevice && $selectedDevice->id === $device->id ? 'active' : '' }}" href="{{ route('admin.index', ['tab' => 'devices', 'device_id' => $device->id]) }}">
                            {{ $device->device_name }} <small>{{ $device->fingerprint }}</small>
                            <span class="pill {{ $device->status === 'banned' ? 'warn' : '' }}">{{ $device->status }}</span>
                            <br><small>{{ $device->user?->email }}</small>
                        </a>
                    @endforeach
                @endif
            </div>
        </div>

        <div class="card">
            @if($tab === 'users' && $selectedUser)
                @php
                    $aclRules = $selectedUser->policy_acl['allow'] ?? [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']];
                    $routeMode = $selectedUser->policy_routes['mode'] ?? 'all';
                    $routes = $selectedUser->policy_routes['include'] ?? ['0.0.0.0/1', '128.0.0.0/1'];
                    $dns = $selectedUser->policy_dns['servers'] ?? ['1.1.1.1', '8.8.8.8'];
                    $selectedRoleIds = $selectedUser->roles->pluck('id')->map(fn($v) => (int) $v)->all();
                @endphp
                <h2>用户策略编辑：{{ $selectedUser->email }}</h2>
                <form method="POST" action="{{ route('admin.users.policy', $selectedUser) }}" data-policy-form>
                    @csrf
                    <div class="editor">
                        <div class="subcard">
                            <label><input type="checkbox" name="is_admin" value="1" {{ $selectedUser->is_admin ? 'checked' : '' }}> 管理员权限</label>
                            <label>角色分配（RBAC）</label>
                            <div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;margin-bottom:8px;">
                                @foreach(($roles ?? []) as $role)
                                    <label style="display:flex;align-items:center;gap:6px;margin:0;">
                                        <input
                                            type="checkbox"
                                            name="role_ids[]"
                                            value="{{ $role->id }}"
                                            {{ in_array((int) $role->id, $selectedRoleIds, true) ? 'checked' : '' }}
                                            style="width:auto;margin:0;"
                                        >
                                        <span>{{ $role->display_name ?: $role->name }}</span>
                                    </label>
                                @endforeach
                            </div>
                            <label>高危确认（仅在变更管理员权限时需要输入一次性确认码）</label>
                            <input name="operation_token" placeholder="例如 ABC-DEF">
                            <label>路由模式</label>
                            <select name="route_mode">
                                @foreach(['all' => '全局', 'split' => '分流', 'custom' => '自定义'] as $k => $v)
                                    <option value="{{ $k }}" {{ $routeMode === $k ? 'selected' : '' }}>{{ $v }}</option>
                                @endforeach
                            </select>
                            <label>路由列表（每行一条）</label>
                            <textarea name="routes_text">@foreach($routes as $r){{ $r."\n" }}@endforeach</textarea>
                            <div data-routes-hidden>@foreach($routes as $r)<input type="hidden" name="routes[]" value="{{ $r }}">@endforeach</div>
                            <label>DNS 列表（每行一条）</label>
                            <textarea name="dns_text">@foreach($dns as $d){{ $d."\n" }}@endforeach</textarea>
                            <div data-dns-hidden>@foreach($dns as $d)<input type="hidden" name="dns_servers[]" value="{{ $d }}">@endforeach</div>
                        </div>
                        <div class="subcard">
                            <label>ACL 规则（CIDR / 协议 / 端口）</label>
                            <div data-acl-list>
                                @foreach($aclRules as $idx => $rule)
                                    <div class="aclrow" data-acl-row>
                                        <input name="acl_rules[{{ $idx }}][cidr]" value="{{ $rule['cidr'] ?? '' }}" placeholder="10.0.0.0/24">
                                        <select name="acl_rules[{{ $idx }}][protocol]">
                                            @foreach(['any','tcp','udp','icmp'] as $p)<option value="{{ $p }}" {{ ($rule['protocol'] ?? 'any') === $p ? 'selected' : '' }}>{{ $p }}</option>@endforeach
                                        </select>
                                        <input name="acl_rules[{{ $idx }}][port]" value="{{ $rule['port'] ?? 'any' }}" placeholder="443/any">
                                        <button type="button" data-remove-row>x</button>
                                    </div>
                                @endforeach
                            </div>
                            <button class="smallbtn" type="button" data-add-acl>+ 增加 ACL 行</button>
                            <label style="margin-top:10px;">JSON 预览（只读）</label>
                            <pre data-json-preview></pre>
                        </div>
                    </div>
                    <button class="save" type="submit" style="margin-top:12px;">保存用户策略</button>
                </form>
                <form method="POST" action="{{ route('admin.users.force-logout', $selectedUser) }}" style="margin-top:12px;" onsubmit="return confirm('确认强制下线该用户当前会话？');">
                    @csrf
                    <div class="subcard">
                        <label>强制下线确认码（一次性确认码）</label>
                        <input name="operation_token" placeholder="例如 ABC-DEF">
                        <button class="smallbtn" type="submit">立即强制下线该用户</button>
                    </div>
                </form>
            @endif

            @if($tab === 'devices' && $selectedDevice)
                @php
                    $aclRules = $selectedDevice->policy_acl['allow'] ?? [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']];
                    $routeMode = $selectedDevice->policy_routes['mode'] ?? 'all';
                    $routes = $selectedDevice->policy_routes['include'] ?? ['0.0.0.0/1', '128.0.0.0/1'];
                    $dns = $selectedDevice->policy_dns['servers'] ?? ['1.1.1.1', '8.8.8.8'];
                @endphp
                <h2>设备策略编辑：{{ $selectedDevice->device_name }}</h2>
                <form method="POST" action="{{ route('admin.devices.policy', $selectedDevice) }}" data-policy-form>
                    @csrf
                    <div class="editor">
                        <div class="subcard">
                            <label>设备状态</label>
                            <select name="status">@foreach(['active','disabled','banned','pending'] as $status)<option value="{{ $status }}" {{ $selectedDevice->status === $status ? 'selected' : '' }}>{{ $status }}</option>@endforeach</select>
                            <label>高危确认（状态设为 disabled/banned 时需输入一次性确认码）</label>
                            <input name="operation_token" placeholder="例如 ABC-DEF">
                            <label>路由模式</label>
                            <select name="route_mode">@foreach(['all' => '全局', 'split' => '分流', 'custom' => '自定义'] as $k => $v)<option value="{{ $k }}" {{ $routeMode === $k ? 'selected' : '' }}>{{ $v }}</option>@endforeach</select>
                            <label>路由列表（每行一条）</label>
                            <textarea name="routes_text">@foreach($routes as $r){{ $r."\n" }}@endforeach</textarea>
                            <div data-routes-hidden>@foreach($routes as $r)<input type="hidden" name="routes[]" value="{{ $r }}">@endforeach</div>
                            <label>DNS 列表（每行一条）</label>
                            <textarea name="dns_text">@foreach($dns as $d){{ $d."\n" }}@endforeach</textarea>
                            <div data-dns-hidden>@foreach($dns as $d)<input type="hidden" name="dns_servers[]" value="{{ $d }}">@endforeach</div>
                        </div>
                        <div class="subcard">
                            <label>ACL 规则（CIDR / 协议 / 端口）</label>
                            <div data-acl-list>
                                @foreach($aclRules as $idx => $rule)
                                    <div class="aclrow" data-acl-row>
                                        <input name="acl_rules[{{ $idx }}][cidr]" value="{{ $rule['cidr'] ?? '' }}" placeholder="10.0.0.0/24">
                                        <select name="acl_rules[{{ $idx }}][protocol]">@foreach(['any','tcp','udp','icmp'] as $p)<option value="{{ $p }}" {{ ($rule['protocol'] ?? 'any') === $p ? 'selected' : '' }}>{{ $p }}</option>@endforeach</select>
                                        <input name="acl_rules[{{ $idx }}][port]" value="{{ $rule['port'] ?? 'any' }}" placeholder="443/any">
                                        <button type="button" data-remove-row>x</button>
                                    </div>
                                @endforeach
                            </div>
                            <button class="smallbtn" type="button" data-add-acl>+ 增加 ACL 行</button>
                            <label style="margin-top:10px;">JSON 预览（只读）</label>
                            <pre data-json-preview></pre>
                        </div>
                    </div>
                    <button class="save" type="submit" style="margin-top:12px;">保存设备策略</button>
                </form>
            @endif
        </div>
    </div>
    @endif
</div>
<div class="modal" id="audit-modal" aria-hidden="true">
    <div class="modal-card">
        <div class="modal-top">
            <h3 style="margin:0;">审计详情</h3>
            <button class="smallbtn" type="button" id="audit-modal-close">关闭</button>
        </div>
        <div class="modal-grid">
            <div class="kv">
                <h4>基础信息</h4>
                <div id="audit-base"></div>
            </div>
            <div class="kv">
                <h4>Metadata 结构</h4>
                <pre id="audit-metadata-pre"></pre>
            </div>
            <div class="kv" style="grid-column:1 / -1;">
                <h4>策略变更 Diff</h4>
                <pre id="audit-diff-pre"></pre>
            </div>
        </div>
    </div>
</div>
<script>
const tokenCard = document.querySelector('[data-token-card]');
const copyTokenBtn = document.getElementById('copy-operation-token');
const tokenTextEl = document.getElementById('operation-token-text');
const tokenCountdownEl = document.getElementById('operation-token-countdown');

function formatRemainingSeconds(seconds) {
    const clamped = Math.max(0, seconds);
    const mins = Math.floor(clamped / 60);
    const secs = clamped % 60;
    return `${mins}分${String(secs).padStart(2, '0')}秒`;
}

if (tokenCard && tokenCountdownEl) {
    const expireAtRaw = tokenCard.getAttribute('data-token-expire') || '';
    const expireAtMs = Date.parse(expireAtRaw);
    if (!Number.isNaN(expireAtMs)) {
        const timer = window.setInterval(() => {
            const remainingSec = Math.floor((expireAtMs - Date.now()) / 1000);
            if (remainingSec <= 0) {
                tokenCountdownEl.textContent = '已过期，请重新生成确认码';
                if (copyTokenBtn) copyTokenBtn.disabled = true;
                window.clearInterval(timer);
                return;
            }
            tokenCountdownEl.textContent = `剩余有效时间：${formatRemainingSeconds(remainingSec)}`;
        }, 1000);
    } else {
        tokenCountdownEl.textContent = '无法解析过期时间';
    }
}

copyTokenBtn?.addEventListener('click', async () => {
    const token = tokenTextEl?.textContent?.trim() || '';
    if (!token) return;
    try {
        if (navigator.clipboard?.writeText) {
            await navigator.clipboard.writeText(token);
        } else {
            const temp = document.createElement('textarea');
            temp.value = token;
            document.body.appendChild(temp);
            temp.select();
            document.execCommand('copy');
            temp.remove();
        }
        copyTokenBtn.textContent = '已复制';
        window.setTimeout(() => {
            copyTokenBtn.textContent = '一键复制';
        }, 1500);
    } catch {
        copyTokenBtn.textContent = '复制失败';
    }
});

const archiveForm = document.querySelector('[data-archive-form]');
archiveForm?.addEventListener('submit', () => {
    const submitBtn = archiveForm.querySelector('[data-archive-submit]');
    if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = '处理中...';
    }
});

document.querySelectorAll('[data-policy-form]').forEach((form) => {
    const aclList = form.querySelector('[data-acl-list]');
    const preview = form.querySelector('[data-json-preview]');
    const routesText = form.querySelector('textarea[name="routes_text"]');
    const dnsText = form.querySelector('textarea[name="dns_text"]');
    const routesHidden = form.querySelector('[data-routes-hidden]');
    const dnsHidden = form.querySelector('[data-dns-hidden]');

    function syncHidden() {
        routesHidden.innerHTML = '';
        dnsHidden.innerHTML = '';
        routesText.value.split('\n').map(v => v.trim()).filter(Boolean).forEach((route) => {
            const input = document.createElement('input');
            input.type = 'hidden';
            input.name = 'routes[]';
            input.value = route;
            routesHidden.appendChild(input);
        });
        dnsText.value.split('\n').map(v => v.trim()).filter(Boolean).forEach((dns) => {
            const input = document.createElement('input');
            input.type = 'hidden';
            input.name = 'dns_servers[]';
            input.value = dns;
            dnsHidden.appendChild(input);
        });
    }

    function refreshPreview() {
        syncHidden();
        const acl = Array.from(aclList.querySelectorAll('[data-acl-row]')).map((row) => ({
            cidr: row.querySelector('input[name*="[cidr]"]').value.trim(),
            protocol: row.querySelector('select[name*="[protocol]"]').value,
            port: row.querySelector('input[name*="[port]"]').value.trim() || 'any',
        })).filter((x) => x.cidr);
        const json = {
            acl: { allow: acl },
            routes: { mode: form.querySelector('select[name="route_mode"]').value, include: routesText.value.split('\n').map(v => v.trim()).filter(Boolean) },
            dns: { servers: dnsText.value.split('\n').map(v => v.trim()).filter(Boolean) }
        };
        preview.textContent = JSON.stringify(json, null, 2);
    }

    form.querySelector('[data-add-acl]').addEventListener('click', () => {
        const idx = aclList.querySelectorAll('[data-acl-row]').length;
        const row = document.createElement('div');
        row.className = 'aclrow';
        row.setAttribute('data-acl-row', '1');
        row.innerHTML = `<input name="acl_rules[${idx}][cidr]" placeholder="10.0.0.0/24"><select name="acl_rules[${idx}][protocol]"><option value="any">any</option><option value="tcp">tcp</option><option value="udp">udp</option><option value="icmp">icmp</option></select><input name="acl_rules[${idx}][port]" value="any" placeholder="443/any"><button type="button" data-remove-row>x</button>`;
        aclList.appendChild(row);
        refreshPreview();
    });

    aclList.addEventListener('click', (e) => {
        if (e.target.matches('[data-remove-row]')) {
            e.target.closest('[data-acl-row]')?.remove();
            refreshPreview();
        }
    });

    form.addEventListener('input', refreshPreview);
    form.addEventListener('submit', syncHidden);
    refreshPreview();
});

const modal = document.getElementById('audit-modal');
const closeBtn = document.getElementById('audit-modal-close');
const baseEl = document.getElementById('audit-base');
const metadataEl = document.getElementById('audit-metadata-pre');
const diffEl = document.getElementById('audit-diff-pre');

function closeAuditModal() {
    modal.classList.remove('show');
    modal.setAttribute('aria-hidden', 'true');
}

document.querySelectorAll('[data-audit-open]').forEach((btn) => {
    btn.addEventListener('click', () => {
        openAuditModalFromButton(btn);
    });
});

function openAuditModalFromButton(btn) {
    const metadata = (() => {
        try { return JSON.parse(btn.dataset.metadata || '{}'); } catch { return {}; }
    })();
    baseEl.innerHTML = `
        <div><strong>时间：</strong>${btn.dataset.created || '-'}</div>
        <div><strong>事件：</strong>${btn.dataset.event || '-'}</div>
        <div><strong>操作人：</strong>${btn.dataset.operator || '-'}</div>
        <div><strong>目标：</strong>user:${btn.dataset.targetUser || '-'} / device:${btn.dataset.targetDevice || '-'}</div>
        <div><strong>IP：</strong>${btn.dataset.ip || '-'}</div>
        <div><strong>说明：</strong>${btn.dataset.message || '-'}</div>
    `;
    metadataEl.textContent = JSON.stringify(metadata, null, 2);
    diffEl.innerHTML = renderDiff(metadata.before ?? null, metadata.after ?? null);
    modal.classList.add('show');
    modal.setAttribute('aria-hidden', 'false');
}

closeBtn?.addEventListener('click', closeAuditModal);
modal?.addEventListener('click', (e) => {
    if (e.target === modal) closeAuditModal();
});

function renderDiff(before, after) {
    if (!before && !after) {
        return '无 before/after 数据（旧审计记录可能未包含差异信息）';
    }
    const rows = [];
    walkDiff(before ?? {}, after ?? {}, '', rows);
    if (rows.length === 0) {
        return '未检测到字段变化';
    }
    return rows.join('\n');
}

function walkDiff(before, after, path, rows) {
    const keys = new Set([
        ...Object.keys(before || {}),
        ...Object.keys(after || {}),
    ]);
    keys.forEach((key) => {
        const currentPath = path ? `${path}.${key}` : key;
        const b = before ? before[key] : undefined;
        const a = after ? after[key] : undefined;

        if (isObject(b) && isObject(a)) {
            walkDiff(b, a, currentPath, rows);
            return;
        }

        if (JSON.stringify(b) !== JSON.stringify(a)) {
            rows.push(`<span class="diff-path">${escapeHtml(currentPath)}</span>`);
            rows.push(`<span class="diff-removed">- ${escapeHtml(stringifyValue(b))}</span>`);
            rows.push(`<span class="diff-added">+ ${escapeHtml(stringifyValue(a))}</span>`);
            rows.push('');
        }
    });
}

function isObject(v) {
    return v !== null && typeof v === 'object' && !Array.isArray(v);
}

function stringifyValue(v) {
    if (typeof v === 'undefined') return 'undefined';
    if (v === null) return 'null';
    if (typeof v === 'string') return v;
    return JSON.stringify(v);
}

function escapeHtml(str) {
    return String(str)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;');
}
</script>
</body>
</html>
