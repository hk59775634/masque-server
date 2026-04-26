<?php

namespace App\Console\Commands;

use App\Models\AuditLog;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('audit:backfill-chain {--force-rehash : Re-hash all records, not only missing hashes}')]
#[Description('Backfill hash-chain fields for historical audit logs')]
class BackfillAuditLogChain extends Command
{
    public function handle(): int
    {
        $forceRehash = (bool) $this->option('force-rehash');
        $processed = 0;
        $updated = 0;
        $prevHash = null;

        AuditLog::query()
            ->orderBy('id')
            ->chunkById(200, function ($logs) use ($forceRehash, &$processed, &$updated, &$prevHash): void {
                foreach ($logs as $log) {
                    $processed++;
                    $expected = AuditLog::calculateHash($prevHash, $log);
                    $needsUpdate = $forceRehash || !$log->entry_hash || $log->prev_hash !== $prevHash || $log->entry_hash !== $expected;

                    if ($needsUpdate) {
                        $log->forceFill([
                            'prev_hash' => $prevHash,
                            'entry_hash' => $expected,
                        ])->saveQuietly();
                        $updated++;
                    }

                    $prevHash = $expected;
                }
            });

        $this->info("Backfill completed. processed={$processed}, updated={$updated}");

        return self::SUCCESS;
    }
}
