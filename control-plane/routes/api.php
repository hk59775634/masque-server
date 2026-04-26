<?php

use App\Http\Controllers\Api\V1\ProvisioningController;
use App\Http\Middleware\EnsureApiIdempotency;
use Illuminate\Support\Facades\Route;

Route::prefix('v1')->middleware('throttle:45,1')->group(function (): void {
    Route::get('/devices/self', [ProvisioningController::class, 'fetchDeviceSelf']);
});

Route::prefix('v1')->middleware('throttle:120,1')->group(function (): void {
    Route::get('/health', static fn () => response()->json([
        'status' => 'ok',
        'service' => 'control-plane',
    ]))->name('api.v1.health');

    Route::post('/users', [ProvisioningController::class, 'createUser'])->middleware(['throttle:10,1', EnsureApiIdempotency::class]);
    Route::post('/users/{user}/policy', [ProvisioningController::class, 'setUserPolicy'])->middleware(['throttle:30,1', EnsureApiIdempotency::class]);
    Route::post('/devices/{device}/policy', [ProvisioningController::class, 'setDevicePolicy'])->middleware(['throttle:30,1', EnsureApiIdempotency::class]);
    Route::post('/devices/activation-code', [ProvisioningController::class, 'issueActivationCode'])->middleware(['throttle:20,1', EnsureApiIdempotency::class]);
    Route::post('/activate', [ProvisioningController::class, 'activateDevice'])->middleware(['throttle:20,1', EnsureApiIdempotency::class]);
    Route::get('/config', [ProvisioningController::class, 'fetchConfig']);
    Route::post('/server/authorize', [ProvisioningController::class, 'authorizeSession'])->middleware(['throttle:240,1', EnsureApiIdempotency::class]);
});
