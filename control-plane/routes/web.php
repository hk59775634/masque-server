<?php

use App\Http\Controllers\Admin\AdminController;
use App\Http\Controllers\Admin\AdminRbacController;
use App\Http\Controllers\Admin\AdminTwoFactorController;
use App\Http\Controllers\Auth\LoginController;
use App\Http\Controllers\Auth\RegisterController;
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
    Route::get('/admin/two-factor/challenge', [AdminTwoFactorController::class, 'challenge'])
        ->name('admin.two-factor.challenge');
    Route::post('/admin/two-factor/challenge', [AdminTwoFactorController::class, 'verify'])
        ->middleware('throttle:20,1')
        ->name('admin.two-factor.verify');

    Route::middleware(['admin.two-factor', EnsureAdminSessionFresh::class])->group(function (): void {
        Route::get('/admin', [AdminController::class, 'index'])->middleware('permission:admin.access')->name('admin.index');
        Route::post('/admin/operation-token', [AdminController::class, 'issueOperationToken'])->middleware('permission:admin.access')->name('admin.operation-token');
        Route::get('/admin/two-factor/setup', [AdminTwoFactorController::class, 'setup'])->name('admin.two-factor.setup');
        Route::post('/admin/two-factor/setup', [AdminTwoFactorController::class, 'confirm'])->name('admin.two-factor.setup.confirm');
        Route::post('/admin/two-factor/disable', [AdminTwoFactorController::class, 'disable'])->name('admin.two-factor.disable');
        Route::post('/admin/users/{user}/force-logout', [AdminController::class, 'forceLogoutUser'])->middleware('permission:admin.session.revoke')->name('admin.users.force-logout');
        Route::post('/admin/users/force-logout-scope', [AdminController::class, 'forceLogoutScope'])->middleware('permission:admin.session.revoke')->name('admin.users.force-logout-scope');
        Route::post('/admin/users/{user}/policy', [AdminController::class, 'updateUserPolicy'])->middleware('permission:admin.policy.write')->name('admin.users.policy');
        Route::post('/admin/devices/{device}/policy', [AdminController::class, 'updateDevicePolicy'])->middleware('permission:admin.policy.write')->name('admin.devices.policy');
        Route::post('/admin/audits/archive-now', [AdminController::class, 'archiveAuditsNow'])->middleware('permission:admin.audit.read')->name('admin.audits.archive-now');
        Route::get('/admin/audits', [AdminController::class, 'index'])->middleware('permission:admin.audit.read')->name('admin.audits');
        Route::get('/admin/audits/export', [AdminController::class, 'exportAudits'])->middleware('permission:admin.audit.read')->name('admin.audits.export');
        Route::get('/admin/rbac', [AdminRbacController::class, 'index'])->middleware('permission:admin.rbac.write')->name('admin.rbac.index');
        Route::post('/admin/rbac/roles', [AdminRbacController::class, 'store'])
            ->middleware(['permission:admin.rbac.write', 'throttle:30,1'])
            ->name('admin.rbac.roles.store');
        Route::post('/admin/rbac/roles/{role}/permissions', [AdminRbacController::class, 'updateRolePermissions'])
            ->middleware(['permission:admin.rbac.write', 'throttle:30,1'])
            ->name('admin.rbac.roles.permissions');
        Route::post('/admin/rbac/users/roles', [AdminRbacController::class, 'syncUserRoles'])
            ->middleware(['permission:admin.rbac.write', 'throttle:30,1'])
            ->name('admin.rbac.users.roles');
    });
});
