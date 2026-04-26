<?php

namespace App\Console\Commands;

use App\Models\AuditLog;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('audit:verify-chain {--from-id=1 : Start verify from audit id}')]
#[Description('Verify append-only hash chain integrity for audit logs')]
class VerifyAuditLogChain extends Command
{
    public function handle(): int
    {
        $fromId = max(1, (int) $this->option('from-id'));
        $processed = 0;
        $broken = 0;
        $prevHash = AuditLog::query()
            ->where('id', '<', $fromId)
            ->latest('id')
            ->value('entry_hash');

        AuditLog::query()
            ->where('id', '>=', $fromId)
            ->orderBy('id')
            ->chunkById(200, function ($logs) use (&$processed, &$broken, &$prevHash) {
                foreach ($logs as $log) {
                    $processed++;
                    $expected = AuditLog::calculateHash($prevHash, $log);
                    if ($log->prev_hash !== $prevHash || $log->entry_hash !== $expected) {
                        $broken++;
                        $this->error("Broken chain at audit_log id={$log->id}");
                        $this->line(" expected_prev={$prevHash}");
                        $this->line(" stored_prev={$log->prev_hash}");
                        $this->line(" expected_hash={$expected}");
                        $this->line(" stored_hash={$log->entry_hash}");
                        break;
                    }
                    $prevHash = $expected;
                }

                if ($broken > 0) {
                    return false;
                }
            });

        if ($broken > 0) {
            $this->error("Verification failed. processed={$processed}, broken={$broken}");
            return self::FAILURE;
        }

        $this->info("Verification passed. processed={$processed}");
        return self::SUCCESS;
    }
}
