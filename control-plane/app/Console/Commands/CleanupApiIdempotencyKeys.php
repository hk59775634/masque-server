<?php

namespace App\Console\Commands;

use App\Models\ApiIdempotencyKey;
use Illuminate\Console\Attributes\Description;
use Illuminate\Console\Attributes\Signature;
use Illuminate\Console\Command;

#[Signature('api:cleanup-idempotency-keys {--hours=72 : Keep records newer than N hours}')]
#[Description('Cleanup stale API idempotency records')]
class CleanupApiIdempotencyKeys extends Command
{
    public function handle(): int
    {
        $hours = max(1, (int) $this->option('hours'));
        $cutoff = now()->subHours($hours);

        $deleted = ApiIdempotencyKey::query()
            ->where(function ($query) use ($cutoff): void {
                $query->where('finished_at', '<', $cutoff)
                    ->orWhere(function ($subQuery) use ($cutoff): void {
                        $subQuery->whereNull('finished_at')
                            ->where('created_at', '<', $cutoff);
                    });
            })
            ->delete();

        $this->info("Idempotency cleanup done. cutoff={$cutoff->toIso8601String()}, deleted={$deleted}");
        return self::SUCCESS;
    }
}
