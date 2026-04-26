<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Symfony\Component\HttpFoundation\Response;

class EnsureSessionNotRevoked
{
    public function handle(Request $request, Closure $next): Response|RedirectResponse
    {
        $user = $request->user();
        if (!$user) {
            return $next($request);
        }

        $sessionIssuedAt = (int) $request->session()->get('auth_issued_at', time());
        $request->session()->put('auth_issued_at', $sessionIssuedAt);

        if ($user->session_invalid_before && $sessionIssuedAt <= $user->session_invalid_before->getTimestamp()) {
            Auth::logout();
            $request->session()->invalidate();
            $request->session()->regenerateToken();

            return redirect()
                ->route('login')
                ->with('status', '会话已被管理员强制下线，请重新登录。');
        }

        return $next($request);
    }
}
