<?php

namespace App\Http\Controllers\Auth;

use App\Http\Controllers\Controller;
use App\Models\AuditLog;
use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\RateLimiter;
use Illuminate\Validation\ValidationException;
use Illuminate\View\View;

class LoginController extends Controller
{
    public function create(): View
    {
        return view('auth.login');
    }

    public function store(Request $request): RedirectResponse
    {
        $credentials = $request->validate([
            'email' => ['required', 'email'],
            'password' => ['required', 'string'],
        ]);

        $emailKey = strtolower(trim($credentials['email']));
        $rateKey = $this->loginRateLimiterKey($request, $emailKey);
        $maxAttempts = max(1, (int) config('security.web_login.max_attempts', 5));
        $decaySeconds = max(60, (int) config('security.web_login.decay_minutes', 15) * 60);

        if (RateLimiter::tooManyAttempts($rateKey, $maxAttempts)) {
            $seconds = RateLimiter::availableIn($rateKey);
            throw ValidationException::withMessages([
                'email' => $this->loginThrottleMessage($seconds),
            ]);
        }

        if (!Auth::attempt([
            'email' => $credentials['email'],
            'password' => $credentials['password'],
        ], $request->boolean('remember'))) {
            RateLimiter::hit($rateKey, $decaySeconds);
            $this->auditWebLoginFailed($request, $emailKey, (int) RateLimiter::attempts($rateKey));

            throw ValidationException::withMessages([
                'email' => '邮箱或密码错误。',
            ]);
        }

        RateLimiter::clear($rateKey);

        $request->session()->regenerate();
        $request->session()->put('auth_issued_at', time());

        $this->auditWebLoginSuccess($request, Auth::user());

        if (
            config('app.env') === 'local'
            && (bool) env('ALLOW_FIRST_USER_ADMIN', true)
            && User::query()->where('is_admin', true)->count() === 0
            && Auth::user()
        ) {
            Auth::user()->update(['is_admin' => true]);
        }

        return redirect()->route('dashboard')->with('status', '登录成功。');
    }

    public function destroy(Request $request): RedirectResponse
    {
        Auth::logout();

        $request->session()->invalidate();
        $request->session()->regenerateToken();

        return redirect('/login')->with('status', '已退出登录。');
    }

    private function loginRateLimiterKey(Request $request, string $emailNormalized): string
    {
        return 'web-login:'.hash('sha256', $emailNormalized.'|'.$request->ip());
    }

    private function loginThrottleMessage(int $seconds): string
    {
        return '登录尝试次数过多，请 '.max(1, $seconds).' 秒后再试。';
    }

    private function auditWebLoginFailed(Request $request, string $emailNormalized, int $attempts): void
    {
        AuditLog::create([
            'user_id' => null,
            'device_id' => null,
            'event_type' => 'auth.web_login_failed',
            'ip_address' => $request->ip(),
            'message' => 'Web login failed (bad credentials)',
            'metadata' => [
                'email' => $emailNormalized,
                'attempts_in_window' => $attempts,
            ],
        ]);
    }

    private function auditWebLoginSuccess(Request $request, User $user): void
    {
        AuditLog::create([
            'user_id' => $user->id,
            'device_id' => null,
            'event_type' => 'auth.web_login_success',
            'ip_address' => $request->ip(),
            'message' => 'Web login succeeded',
            'metadata' => [
                'email' => $user->email,
            ],
        ]);
    }
}
