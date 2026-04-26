<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Model;

class Device extends Model
{
    protected $fillable = [
        'user_id',
        'device_name',
        'fingerprint',
        'status',
        'api_token_hash',
        'policy_acl',
        'policy_routes',
        'policy_dns',
        'last_seen_at',
        'token_expires_at',
    ];

    protected $casts = [
        'policy_acl' => 'array',
        'policy_routes' => 'array',
        'policy_dns' => 'array',
        'last_seen_at' => 'datetime',
        'token_expires_at' => 'datetime',
    ];

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }
}
