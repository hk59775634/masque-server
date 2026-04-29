<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class EnsureUserHasPermission
{
    public function handle(Request $request, Closure $next, string $permission): Response
    {
        $user = $request->user();
        if (! $user) {
            abort(401, 'Authentication required');
        }
        if (! $user->hasPermission($permission)) {
            abort(403, "Permission '{$permission}' required");
        }

        return $next($request);
    }
}

