<?php

namespace App\Http\Controllers\Admin;

use App\Http\Controllers\Controller;
use App\Models\AdminOperationToken;
use App\Models\AuditLog;
use App\Models\Device;
use App\Models\User;
use App\Services\MasquePrometheusMetrics;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Validation\ValidationException;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Hash;
use Illuminate\Support\Str;
use Throwable;
use Symfony\Component\HttpFoundation\StreamedResponse;
use Illuminate\View\View;

class AdminController extends Controller
{
    public function index(Request $request): View
    {
        $this->authorizeAdmin($request);
        $users = User::query()->orderBy('id')->get();
        $devices = Device::query()->with('user')->orderByDesc('id')->get();
        $audits = $this->filteredAuditQuery($request)
            ->latest()
            ->paginate(30)
            ->appends($request->query());
        $includeArchived = $request->boolean('include_archived');
        $auditEventTypes = AuditLog::query()
            ->when(!$includeArchived, function ($query): void {
                $query->whereNull('archived_at');
            })
            ->select('event_type')
            ->distinct()
            ->orderBy('event_type')
            ->pluck('event_type');
        $auditStats = [
            'active_count' => AuditLog::query()->whereNull('archived_at')->count(),
            'archived_count' => AuditLog::query()->whereNotNull('archived_at')->count(),
            'last_archived_at' => AuditLog::query()->whereNotNull('archived_at')->max('archived_at'),
        ];
        $archiveRuns = AuditLog::query()
            ->where('event_type', 'admin.audit_archive_run')
            ->latest()
            ->limit(10)
            ->get();
        $lastArchiveRun = $archiveRuns->first();
        $archiveJobRunning = Cache::has('admin:audit-archive:running');

        $grafana = rtrim((string) config('services.observability.grafana_url'), '/');
        $prometheusUi = rtrim((string) config('services.observability.prometheus_ui_url'), '/');
        $alertmanager = rtrim((string) config('services.observability.alertmanager_url'), '/');
        $loki = rtrim((string) config('services.observability.loki_url'), '/');

        $opsOverview = [
            'db' => [
                'users_total' => User::query()->count(),
                'devices_total' => Device::query()->count(),
                'devices_seen_24h' => Device::query()->where('last_seen_at', '>=', now()->subDay())->count(),
                'devices_seen_7d' => Device::query()->where('last_seen_at', '>=', now()->subDays(7))->count(),
            ],
            'prometheus' => app(MasquePrometheusMetrics::class)->snapshot(),
            'ui' => [
                'grafana' => $grafana,
                'grafana_explore' => $grafana !== '' ? $grafana.'/explore' : '',
                'prometheus' => $prometheusUi,
                'alertmanager' => $alertmanager,
                'alertmanager_silences' => $alertmanager !== '' ? $alertmanager.'/#/silences' : '',
                'alertmanager_silence_new' => $alertmanager !== '' ? $alertmanager.'/#/silences/new' : '',
                'loki' => $loki,
                'loki_ready' => $loki !== '' ? $loki.'/ready' : '',
                'grafana_loki_cheatsheet' => $grafana !== '' ? $grafana.'/d/afbuyers-loki-cheatsheet/loki-logql-cheatsheet' : '',
            ],
        ];

        return view('admin.index', [
            'users' => $users,
            'devices' => $devices,
            'audits' => $audits,
            'auditEventTypes' => $auditEventTypes,
            'selectedUserId' => (int) $request->query('user_id', 0),
            'selectedDeviceId' => (int) $request->query('device_id', 0),
            'tab' => (string) $request->query('tab', 'users'),
            'includeArchived' => $includeArchived,
            'auditStats' => $auditStats,
            'archiveRuns' => $archiveRuns,
            'archiveLastRun' => $lastArchiveRun,
            'archiveJobRunning' => $archiveJobRunning,
            'operationToken' => (string) $request->session()->get('operation_token', ''),
            'operationTokenExpiresAt' => (string) $request->session()->get('operation_token_expires_at', ''),
            'opsOverview' => $opsOverview,
        ]);
    }

    public function issueOperationToken(Request $request): RedirectResponse
    {
        $this->authorizeAdmin($request);

        $plainToken = Str::upper(Str::random(3).'-'.Str::random(3));
        $expiresAt = now()->addMinutes(5);

        AdminOperationToken::create([
            'user_id' => (int) $request->user()->id,
            'purpose' => 'high_risk_admin_action',
            'token_hash' => Hash::make($plainToken),
            'expires_at' => $expiresAt,
        ]);

        $this->writeAudit(
            $request,
            'admin.operation_token_issued',
            'Issued one-time admin confirmation token',
            (int) $request->user()->id,
            null,
            ['expires_at' => $expiresAt->toIso8601String()]
        );

        return redirect()
            ->route('admin.index', ['tab' => (string) $request->input('tab', 'users')])
            ->with('operation_token', $plainToken)
            ->with('operation_token_expires_at', $expiresAt->toIso8601String())
            ->with('status', '一次性确认码已生成（5分钟有效）。');
    }

    public function exportAudits(Request $request): StreamedResponse
    {
        $this->authorizeAdmin($request);
        $logs = $this->filteredAuditQuery($request)->latest()->limit(5000)->get();

        return response()->streamDownload(function () use ($logs): void {
            $out = fopen('php://output', 'w');
            fputcsv($out, ['created_at', 'event_type', 'operator_email', 'user_id', 'device_id', 'ip_address', 'message']);
            foreach ($logs as $log) {
                fputcsv($out, [
                    (string) $log->created_at,
                    $log->event_type,
                    (string) ($log->metadata['operator_email'] ?? ''),
                    (string) ($log->user_id ?? ''),
                    (string) ($log->device_id ?? ''),
                    (string) ($log->ip_address ?? ''),
                    $log->message,
                ]);
            }
            fclose($out);
        }, 'audit-logs.csv', ['Content-Type' => 'text/csv']);
    }

    public function updateUserPolicy(Request $request, User $user): RedirectResponse
    {
        $this->authorizeAdmin($request);
        $payload = $request->validate([
            'is_admin' => ['nullable', 'in:1'],
            'operation_token' => ['nullable', 'string', 'max:32'],
            'route_mode' => ['required', 'in:all,split,custom'],
            'routes' => ['nullable', 'array'],
            'routes.*' => ['nullable', 'string', 'max:50'],
            'dns_servers' => ['nullable', 'array'],
            'dns_servers.*' => ['nullable', 'ip'],
            'acl_rules' => ['nullable', 'array'],
            'acl_rules.*.cidr' => ['nullable', 'string', 'max:50'],
            'acl_rules.*.protocol' => ['nullable', 'in:any,tcp,udp,icmp'],
            'acl_rules.*.port' => ['nullable', 'string', 'max:30'],
        ]);

        $policyAcl = ['allow' => $this->sanitizeAclRules($payload['acl_rules'] ?? [])];
        $policyRoutes = ['mode' => $payload['route_mode'], 'include' => $this->cleanStringArray($payload['routes'] ?? [])];
        $policyDns = ['servers' => $this->cleanStringArray($payload['dns_servers'] ?? [])];
        $afterIsAdmin = $request->boolean('is_admin');
        $isHighRisk = ($user->is_admin !== $afterIsAdmin);
        $this->assertHighRiskConfirmed($request, $isHighRisk, $payload['operation_token'] ?? null);
        $before = [
            'policy_acl' => $user->policy_acl,
            'policy_routes' => $user->policy_routes,
            'policy_dns' => $user->policy_dns,
            'is_admin' => $user->is_admin,
        ];
        $after = [
            'policy_acl' => $policyAcl,
            'policy_routes' => $policyRoutes,
            'policy_dns' => $policyDns,
            'is_admin' => $afterIsAdmin,
        ];

        $user->update([
            'policy_acl' => $after['policy_acl'],
            'policy_routes' => $after['policy_routes'],
            'policy_dns' => $after['policy_dns'],
            'is_admin' => $after['is_admin'],
        ]);
        $this->writeAudit($request, 'admin.user_policy_updated', "Updated user policy: {$user->email}", $user->id, null, [
            'before' => $before,
            'after' => $after,
        ]);

        return redirect()
            ->route('admin.index', ['tab' => 'users', 'user_id' => $user->id])
            ->with('status', '用户策略已更新。');
    }

    public function forceLogoutUser(Request $request, User $user): RedirectResponse
    {
        $this->authorizeAdmin($request);
        $payload = $request->validate([
            'operation_token' => ['nullable', 'string', 'max:32'],
        ]);
        $this->assertHighRiskConfirmed($request, true, $payload['operation_token'] ?? null);

        $before = [
            'session_invalid_before' => $user->session_invalid_before?->toIso8601String(),
        ];
        $now = now();
        $user->update([
            'session_invalid_before' => $now,
        ]);

        $this->writeAudit(
            $request,
            'admin.user_force_logout',
            "Forced logout user: {$user->email}",
            $user->id,
            null,
            [
                'before' => $before,
                'after' => ['session_invalid_before' => $now->toIso8601String()],
            ]
        );

        return redirect()
            ->route('admin.index', ['tab' => 'users', 'user_id' => $user->id])
            ->with('status', "已强制下线用户 {$user->email}。");
    }

    public function forceLogoutScope(Request $request): RedirectResponse
    {
        $this->authorizeAdmin($request);
        $payload = $request->validate([
            'operation_token' => ['nullable', 'string', 'max:32'],
            'scope' => ['required', 'in:all_users,non_admin_users'],
        ]);
        $this->assertHighRiskConfirmed($request, true, $payload['operation_token'] ?? null);

        $operatorId = (int) $request->user()->id;
        $scope = (string) $payload['scope'];
        $query = User::query()->where('id', '!=', $operatorId);
        if ($scope === 'non_admin_users') {
            $query->where('is_admin', false);
        }

        $affectedCount = $query->count();
        $now = now();
        $query->update(['session_invalid_before' => $now]);

        $this->writeAudit(
            $request,
            'admin.user_force_logout_scope',
            "Forced logout by scope: {$scope}, affected={$affectedCount}",
            null,
            null,
            [
                'scope' => $scope,
                'affected_count' => $affectedCount,
                'excluded_operator_user_id' => $operatorId,
                'after' => ['session_invalid_before' => $now->toIso8601String()],
            ]
        );

        return redirect()
            ->route('admin.index', ['tab' => 'users'])
            ->with('status', "已执行批量强制下线：{$scope}（影响 {$affectedCount} 个账号）。");
    }

    public function archiveAuditsNow(Request $request): RedirectResponse
    {
        $this->authorizeAdmin($request);
        $payload = $request->validate([
            'operation_token' => ['nullable', 'string', 'max:32'],
            'days' => ['nullable', 'integer', 'min:30', 'max:3650'],
        ]);
        $this->assertHighRiskConfirmed($request, true, $payload['operation_token'] ?? null);

        $days = (int) ($payload['days'] ?? 180);
        $lockKey = sprintf('admin:audit-archive:%d:%d', (int) $request->user()->id, $days);
        if (!Cache::add($lockKey, '1', now()->addSeconds(20))) {
            return redirect()
                ->route('admin.audits', ['tab' => 'audits', 'include_archived' => 1])
                ->with('status', '归档任务正在处理中，请勿重复提交。');
        }
        Cache::put('admin:audit-archive:running', '1', now()->addSeconds(20));

        $cutoff = now()->subDays($days);
        try {
            $archivedCount = AuditLog::query()
                ->whereNull('archived_at')
                ->where('created_at', '<', $cutoff)
                ->update(['archived_at' => now()]);

            $this->writeAudit(
                $request,
                'admin.audit_archive_run',
                "Manual audit archive executed: days={$days}, archived={$archivedCount}",
                null,
                null,
                [
                    'days' => $days,
                    'archived_count' => $archivedCount,
                    'cutoff' => $cutoff->toIso8601String(),
                    'success' => true,
                ]
            );
            $statusMessage = "已执行审计归档：归档 {$archivedCount} 条（阈值 {$days} 天）。";
        } catch (Throwable $e) {
            $this->writeAudit(
                $request,
                'admin.audit_archive_run',
                "Manual audit archive failed: days={$days}, error={$e->getMessage()}",
                null,
                null,
                [
                    'days' => $days,
                    'archived_count' => 0,
                    'cutoff' => $cutoff->toIso8601String(),
                    'success' => false,
                    'error' => $e->getMessage(),
                ]
            );
            $statusMessage = '执行审计归档失败，请查看审计详情中的错误信息。';
        } finally {
            Cache::forget($lockKey);
            Cache::forget('admin:audit-archive:running');
        }

        return redirect()
            ->route('admin.audits', ['tab' => 'audits', 'include_archived' => 1])
            ->with('status', $statusMessage);
    }

    public function updateDevicePolicy(Request $request, Device $device): RedirectResponse
    {
        $this->authorizeAdmin($request);
        $validated = $request->validate([
            'status' => ['required', 'in:active,disabled,banned,pending'],
            'operation_token' => ['nullable', 'string', 'max:32'],
            'route_mode' => ['required', 'in:all,split,custom'],
            'routes' => ['nullable', 'array'],
            'routes.*' => ['nullable', 'string', 'max:50'],
            'dns_servers' => ['nullable', 'array'],
            'dns_servers.*' => ['nullable', 'ip'],
            'acl_rules' => ['nullable', 'array'],
            'acl_rules.*.cidr' => ['nullable', 'string', 'max:50'],
            'acl_rules.*.protocol' => ['nullable', 'in:any,tcp,udp,icmp'],
            'acl_rules.*.port' => ['nullable', 'string', 'max:30'],
        ]);

        $policyAcl = ['allow' => $this->sanitizeAclRules($validated['acl_rules'] ?? [])];
        $policyRoutes = ['mode' => $validated['route_mode'], 'include' => $this->cleanStringArray($validated['routes'] ?? [])];
        $policyDns = ['servers' => $this->cleanStringArray($validated['dns_servers'] ?? [])];
        $isHighRisk = in_array($validated['status'], ['disabled', 'banned'], true);
        $this->assertHighRiskConfirmed($request, $isHighRisk, $validated['operation_token'] ?? null);
        $before = [
            'status' => $device->status,
            'policy_acl' => $device->policy_acl,
            'policy_routes' => $device->policy_routes,
            'policy_dns' => $device->policy_dns,
        ];
        $after = [
            'status' => $validated['status'],
            'policy_acl' => $policyAcl,
            'policy_routes' => $policyRoutes,
            'policy_dns' => $policyDns,
        ];

        $device->update([
            'status' => $after['status'],
            'policy_acl' => $after['policy_acl'],
            'policy_routes' => $after['policy_routes'],
            'policy_dns' => $after['policy_dns'],
        ]);
        $this->writeAudit($request, 'admin.device_policy_updated', "Updated device policy: {$device->device_name}", $device->user_id, $device->id, [
            'before' => $before,
            'after' => $after,
        ]);

        return redirect()
            ->route('admin.index', ['tab' => 'devices', 'device_id' => $device->id])
            ->with('status', '设备策略和状态已更新。');
    }

    private function authorizeAdmin(Request $request): void
    {
        abort_unless($request->user()?->is_admin, 403, 'Admin access required');
    }

    private function sanitizeAclRules(array $rules): array
    {
        $clean = [];
        foreach ($rules as $rule) {
            $cidr = trim((string) ($rule['cidr'] ?? ''));
            if ($cidr === '') {
                continue;
            }
            $clean[] = [
                'cidr' => $cidr,
                'protocol' => (string) ($rule['protocol'] ?? 'any'),
                'port' => trim((string) ($rule['port'] ?? 'any')) ?: 'any',
            ];
        }

        return $clean;
    }

    private function cleanStringArray(array $items): array
    {
        return array_values(array_filter(array_map(
            static fn ($item): string => trim((string) $item),
            $items
        )));
    }

    private function writeAudit(
        Request $request,
        string $eventType,
        string $message,
        ?int $targetUserId,
        ?int $targetDeviceId,
        array $payload
    ): void {
        AuditLog::create([
            'user_id' => $targetUserId,
            'device_id' => $targetDeviceId,
            'event_type' => $eventType,
            'ip_address' => $request->ip(),
            'message' => $message,
            'metadata' => array_merge($payload, [
                'operator_user_id' => $request->user()?->id,
                'operator_email' => $request->user()?->email,
            ]),
        ]);
    }

    private function assertHighRiskConfirmed(Request $request, bool $isHighRisk, ?string $operationToken): void
    {
        if (!$isHighRisk) {
            return;
        }

        $rawToken = trim((string) $operationToken);
        if ($rawToken === '') {
            throw ValidationException::withMessages([
                'operation_token' => '高危操作需要填写一次性确认码。',
            ]);
        }

        $candidateTokens = AdminOperationToken::query()
            ->where('user_id', (int) $request->user()->id)
            ->where('purpose', 'high_risk_admin_action')
            ->whereNull('used_at')
            ->where('expires_at', '>', now())
            ->latest()
            ->limit(10)
            ->get();

        $matchedToken = $candidateTokens->first(
            fn (AdminOperationToken $token): bool => Hash::check($rawToken, $token->token_hash)
        );

        if (!$matchedToken) {
            throw ValidationException::withMessages([
                'operation_token' => '一次性确认码无效或已过期。',
            ]);
        }

        $matchedToken->update(['used_at' => now()]);

        $this->writeAudit(
            $request,
            'admin.high_risk_confirmed',
            'High-risk operation confirmed by one-time token',
            null,
            null,
            ['operation_token_used' => true]
        );
    }

    private function filteredAuditQuery(Request $request)
    {
        $query = AuditLog::query();
        if (!$request->boolean('include_archived')) {
            $query->whereNull('archived_at');
        }

        $eventType = trim((string) $request->query('event_type', ''));
        if ($eventType !== '') {
            $query->where('event_type', $eventType);
        }

        $userId = (int) $request->query('filter_user_id', 0);
        if ($userId > 0) {
            $query->where('user_id', $userId);
        }

        $deviceId = (int) $request->query('filter_device_id', 0);
        if ($deviceId > 0) {
            $query->where('device_id', $deviceId);
        }

        $operator = trim((string) $request->query('operator_email', ''));
        if ($operator !== '') {
            $query->where('metadata->operator_email', 'like', "%{$operator}%");
        }

        $from = trim((string) $request->query('from', ''));
        if ($from !== '') {
            $query->whereDate('created_at', '>=', $from);
        }

        $to = trim((string) $request->query('to', ''));
        if ($to !== '') {
            $query->whereDate('created_at', '<=', $to);
        }

        return $query;
    }
}
