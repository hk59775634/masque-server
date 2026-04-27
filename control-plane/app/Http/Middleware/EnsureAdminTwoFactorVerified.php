<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class EnsureAdminTwoFactorVerified
{
    public function handle(Request $request, Closure $next): Response
    {
        $user = $request->user();
        if (! $user?->is_admin) {
            return $next($request);
        }

        if ($user->two_factor_confirmed_at === null) {
            return $next($request);
        }

        if ($request->session()->get('admin_two_factor_verified') === true) {
            return $next($request);
        }

        return redirect()->guest(route('admin.two-factor.challenge'));
    }
}
