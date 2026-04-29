<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Symfony\Component\HttpFoundation\Response;

class EnsureAdminSessionFresh
{
    public function handle(Request $request, Closure $next): Response|RedirectResponse
    {
        $user = $request->user();
        if (!$user || !$user->hasPermission('admin.access')) {
            return $next($request);
        }

        $timeoutMinutes = (int) env('ADMIN_SESSION_IDLE_TIMEOUT_MINUTES', 30);
        $lastActivityAt = (int) $request->session()->get('admin_last_activity_at', time());
        $now = time();

        if ($timeoutMinutes > 0) {
            $idleSeconds = $now - $lastActivityAt;
            if ($idleSeconds > ($timeoutMinutes * 60)) {
                Auth::logout();
                $request->session()->invalidate();
                $request->session()->regenerateToken();

                return redirect()
                    ->route('login')
                    ->with('status', '管理员会话因空闲超时已失效，请重新登录。');
            }
        }

        $request->session()->put('admin_last_activity_at', $now);

        return $next($request);
    }
}
