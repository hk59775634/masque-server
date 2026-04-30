<?php

namespace App\Http\Controllers\Admin\Concerns;

use App\Models\AdminOperationToken;
use App\Models\AuditLog;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Hash;
use Illuminate\Validation\ValidationException;

trait ConfirmsHighRiskAdminOperations
{
    protected function writeAudit(
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

    protected function assertHighRiskConfirmed(Request $request, bool $isHighRisk, ?string $operationToken): void
    {
        if (! $isHighRisk) {
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

        if (! $matchedToken) {
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
}
