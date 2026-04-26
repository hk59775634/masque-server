<?php

use App\Http\Controllers\Auth\LoginController;
use App\Http\Controllers\Auth\RegisterController;
use App\Http\Controllers\Admin\AdminController;
use App\Http\Middleware\EnsureAdminSessionFresh;
use App\Http\Middleware\EnsureSessionNotRevoked;
use Illuminate\Support\Facades\Route;

Route::get('/', function () {
    return view('welcome');
});

Route::get('/dashboard', function () {
    return view('dashboard');
})->middleware(['auth', EnsureSessionNotRevoked::class])->name('dashboard');

Route::get('/register', [RegisterController::class, 'create'])
    ->middleware('guest')
    ->name('register');
Route::post('/register', [RegisterController::class, 'store'])
    ->middleware('guest')
    ->middleware('throttle:10,1')
    ->name('register.store');

Route::get('/login', [LoginController::class, 'create'])
    ->middleware('guest')
    ->name('login');
Route::post('/login', [LoginController::class, 'store'])
    ->middleware('guest')
    ->middleware('throttle:10,1')
    ->name('login.store');

Route::post('/logout', [LoginController::class, 'destroy'])
    ->middleware(['auth', EnsureSessionNotRevoked::class])
    ->name('logout');

Route::middleware(['auth', EnsureSessionNotRevoked::class])->group(function (): void {
    Route::middleware(EnsureAdminSessionFresh::class)->group(function (): void {
        Route::get('/admin', [AdminController::class, 'index'])->name('admin.index');
        Route::post('/admin/operation-token', [AdminController::class, 'issueOperationToken'])->name('admin.operation-token');
        Route::post('/admin/users/{user}/force-logout', [AdminController::class, 'forceLogoutUser'])->name('admin.users.force-logout');
        Route::post('/admin/users/force-logout-scope', [AdminController::class, 'forceLogoutScope'])->name('admin.users.force-logout-scope');
        Route::post('/admin/users/{user}/policy', [AdminController::class, 'updateUserPolicy'])->name('admin.users.policy');
        Route::post('/admin/devices/{device}/policy', [AdminController::class, 'updateDevicePolicy'])->name('admin.devices.policy');
        Route::post('/admin/audits/archive-now', [AdminController::class, 'archiveAuditsNow'])->name('admin.audits.archive-now');
        Route::get('/admin/audits', [AdminController::class, 'index'])->name('admin.audits');
        Route::get('/admin/audits/export', [AdminController::class, 'exportAudits'])->name('admin.audits.export');
    });
});
