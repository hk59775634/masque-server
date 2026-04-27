<?php

use App\Http\Middleware\EnsureAdminTwoFactorVerified;
use App\Http\Middleware\SecurityHeaders;
use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;

return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        web: __DIR__.'/../routes/web.php',
        api: __DIR__.'/../routes/api.php',
        commands: __DIR__.'/../routes/console.php',
        health: '/up',
    )
    ->withMiddleware(function (Middleware $middleware): void {
        // Nginx / CDN terminate TLS; trust X-Forwarded-* for client IP and HTTPS detection.
        $middleware->trustProxies(at: '*');
        $middleware->append(SecurityHeaders::class);
        $middleware->alias([
            'admin.two-factor' => EnsureAdminTwoFactorVerified::class,
        ]);
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        //
    })->create();
