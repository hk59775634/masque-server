<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Support\Carbon;

class AuditLog extends Model
{
    protected $fillable = [
        'user_id',
        'device_id',
        'event_type',
        'ip_address',
        'message',
        'metadata',
        'prev_hash',
        'entry_hash',
        'archived_at',
    ];

    protected $casts = [
        'metadata' => 'array',
        'created_at' => 'datetime',
        'updated_at' => 'datetime',
        'archived_at' => 'datetime',
    ];

    protected static function booted(): void
    {
        static::creating(function (AuditLog $log): void {
            $now = Carbon::now();
            $log->created_at = $log->created_at ?? $now;
            $log->updated_at = $log->updated_at ?? $now;

            if (!empty($log->entry_hash)) {
                return;
            }

            $previous = self::query()->select(['id', 'entry_hash'])->latest('id')->first();
            $log->prev_hash = $previous?->entry_hash ?: null;
            $log->entry_hash = self::calculateHash($log->prev_hash, $log);
        });
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    public function device(): BelongsTo
    {
        return $this->belongsTo(Device::class);
    }

    public static function calculateHash(?string $prevHash, AuditLog $log): string
    {
        $payload = implode('|', [
            (string) $prevHash,
            (string) ($log->created_at?->toIso8601String() ?? ''),
            (string) ($log->user_id ?? ''),
            (string) ($log->device_id ?? ''),
            (string) $log->event_type,
            (string) ($log->ip_address ?? ''),
            (string) $log->message,
            self::normalizeMetadata($log->metadata),
        ]);

        return hash_hmac('sha256', $payload, (string) config('app.key'));
    }

    private static function normalizeMetadata(mixed $metadata): string
    {
        if (!is_array($metadata)) {
            return json_encode($metadata, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES) ?: '';
        }

        $sorted = self::sortRecursive($metadata);

        return json_encode($sorted, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES) ?: '';
    }

    private static function sortRecursive(array $input): array
    {
        foreach ($input as $key => $value) {
            if (is_array($value)) {
                $input[$key] = self::sortRecursive($value);
            }
        }

        if (!array_is_list($input)) {
            ksort($input);
        }

        return $input;
    }
}
