<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class ApiIdempotencyKey extends Model
{
    protected $fillable = [
        'idempotency_key',
        'method',
        'path',
        'request_hash',
        'status_code',
        'response_body',
        'finished_at',
    ];

    protected $casts = [
        'finished_at' => 'datetime',
    ];
}
