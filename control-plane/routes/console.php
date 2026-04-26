<?php

use Illuminate\Foundation\Inspiring;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Schedule;

Artisan::command('inspire', function () {
    $this->comment(Inspiring::quote());
})->purpose('Display an inspiring quote');

Schedule::command('api:cleanup-idempotency-keys --hours=72')->dailyAt('03:20');
Schedule::command('audit:archive-old --days=180')->dailyAt('03:35');
