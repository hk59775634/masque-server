<?php

namespace App\Console\Commands;

use App\Models\AuditLog;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('audit:archive-old {--days=180 : Archive logs older than N days}')]
#[Description('Archive old audit logs by setting archived_at timestamp')]
class ArchiveOldAuditLogs extends Command
{
    public function handle(): int
    {
        $days = max(1, (int) $this->option('days'));
        $cutoff = now()->subDays($days);

        $affected = AuditLog::query()
            ->whereNull('archived_at')
            ->where('created_at', '<', $cutoff)
            ->update(['archived_at' => now()]);

        $this->info("Audit archive done. cutoff={$cutoff->toIso8601String()}, archived={$affected}");
        return self::SUCCESS;
    }
}
