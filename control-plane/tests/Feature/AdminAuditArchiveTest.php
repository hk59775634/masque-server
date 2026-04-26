<?php

namespace Tests\Feature;

use App\Models\AdminOperationToken;
use App\Models\AuditLog;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Hash;
use Tests\TestCase;

class AdminAuditArchiveTest extends TestCase
{
    use RefreshDatabase;

    public function test_admin_can_archive_old_audits_and_write_audit_event(): void
    {
        $admin = User::factory()->create([
            'is_admin' => true,
        ]);

        // Older than 180 days: should be archived.
        $oldLog = AuditLog::create([
            'event_type' => 'policy.user.updated',
            'message' => 'old audit',
        ]);
        $oldLog->forceFill([
            'created_at' => now()->subDays(220),
            'updated_at' => now()->subDays(220),
        ])->saveQuietly();

        // Newer than 180 days: should remain active.
        $recentLog = AuditLog::create([
            'event_type' => 'policy.device.updated',
            'message' => 'recent audit',
        ]);
        $recentLog->forceFill([
            'created_at' => now()->subDays(20),
            'updated_at' => now()->subDays(20),
        ])->saveQuietly();

        AdminOperationToken::create([
            'user_id' => $admin->id,
            'purpose' => 'high_risk_admin_action',
            'token_hash' => Hash::make('ABC-DEF'),
            'expires_at' => now()->addMinutes(5),
        ]);

        $response = $this
            ->actingAs($admin)
            ->post(route('admin.audits.archive-now'), [
                'days' => 180,
                'operation_token' => 'ABC-DEF',
            ]);

        $response
            ->assertStatus(302)
            ->assertSessionHas('status');

        $this->assertSame(1, AuditLog::query()->whereNotNull('archived_at')->count());
        $this->assertNull($recentLog->fresh()->archived_at);

        $archiveEvent = AuditLog::query()
            ->where('event_type', 'admin.audit_archive_run')
            ->latest('id')
            ->first();

        $this->assertNotNull($archiveEvent);
        $this->assertSame(180, (int) ($archiveEvent->metadata['days'] ?? 0));
        $this->assertSame(1, (int) ($archiveEvent->metadata['archived_count'] ?? 0));
        $this->assertSame($admin->email, (string) ($archiveEvent->metadata['operator_email'] ?? ''));
    }
}
