<?php

namespace App\Http\Controllers\Admin;

use App\Http\Controllers\Controller;
use App\Models\AdminOperationToken;
use App\Models\AuditLog;
use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Hash;
use Illuminate\Validation\ValidationException;
use Illuminate\View\View;
use PragmaRX\Google2FA\Google2FA;

class AdminTwoFactorController extends Controller
{
    public function challenge(Request $request): View|RedirectResponse
    {
        $user = $request->user();
        abort_unless($user instanceof User, 403);
        abort_unless($user->is_admin, 403);
        abort_unless($user->two_factor_confirmed_at !== null, 404);

        if ($request->session()->get('admin_two_factor_verified') === true) {
            return redirect()->route('admin.index', ['tab' => 'overview']);
        }

        return view('admin.two-factor-challenge');
    }

    public function verify(Request $request, Google2FA $google2fa): RedirectResponse
    {
        $user = $request->user();
        abort_unless($user instanceof User, 403);
        abort_unless($user->is_admin, 403);
        abort_unless($user->two_factor_confirmed_at !== null, 404);

        $validated = $request->validate([
            'code' => ['required', 'string', 'size:6'],
        ]);

        $secret = $user->two_factor_secret;
        if ($secret === null || $secret === '') {
            throw ValidationException::withMessages([
                'code' => '两步验证未正确配置，请联系其他管理员。',
            ]);
        }

        if (! $google2fa->verifyKey($secret, $validated['code'], 2)) {
            throw ValidationException::withMessages([
                'code' => '验证码无效或已过期。',
            ]);
        }

        $request->session()->put('admin_two_factor_verified', true);

        AuditLog::create([
            'user_id' => $user->id,
            'device_id' => null,
            'event_type' => 'auth.admin_two_factor_passed',
            'ip_address' => $request->ip(),
            'message' => 'Admin passed two-factor challenge',
            'metadata' => [],
        ]);

        return redirect()->intended(route('admin.index', ['tab' => 'overview']))
            ->with('status', '两步验证已通过。');
    }

    public function setup(Request $request): View
    {
        $this->authorizeAdmin($request);

        /** @var User $user */
        $user = $request->user();

        if ($user->two_factor_confirmed_at !== null) {
            return view('admin.two-factor-setup', [
                'otpauthUrl' => '',
                'plainSecret' => '',
                'enabled' => true,
            ]);
        }

        $pending = (string) $request->session()->get('two_factor_pending_secret', '');
        if ($pending === '') {
            $pending = (new Google2FA)->generateSecretKey();
            $request->session()->put('two_factor_pending_secret', $pending);
        }

        $issuer = config('app.name', 'MASQUE');
        $otpauth = (new Google2FA)->getQRCodeUrl($issuer, (string) $user->email, $pending);

        return view('admin.two-factor-setup', [
            'otpauthUrl' => $otpauth,
            'plainSecret' => $pending,
            'enabled' => false,
        ]);
    }

    public function confirm(Request $request, Google2FA $google2fa): RedirectResponse
    {
        $this->authorizeAdmin($request);

        $validated = $request->validate([
            'code' => ['required', 'string', 'size:6'],
        ]);

        $pending = (string) $request->session()->get('two_factor_pending_secret', '');
        if ($pending === '') {
            throw ValidationException::withMessages([
                'code' => '会话已过期，请刷新「两步验证」页面后重试。',
            ]);
        }

        if (! $google2fa->verifyKey($pending, $validated['code'], 2)) {
            throw ValidationException::withMessages([
                'code' => '验证码无效，请重试。',
            ]);
        }

        /** @var User $user */
        $user = $request->user();
        $user->two_factor_secret = $pending;
        $user->two_factor_confirmed_at = now();
        $user->save();

        $request->session()->forget('two_factor_pending_secret');
        $request->session()->put('admin_two_factor_verified', true);

        AuditLog::create([
            'user_id' => $user->id,
            'device_id' => null,
            'event_type' => 'auth.admin_two_factor_enabled',
            'ip_address' => $request->ip(),
            'message' => 'Admin enabled two-factor authentication',
            'metadata' => [],
        ]);

        return redirect()
            ->route('admin.two-factor.setup')
            ->with('status', '两步验证已启用。后续登录管理功能前需输入验证码。');
    }

    public function disable(Request $request): RedirectResponse
    {
        $this->authorizeAdmin($request);

        $validated = $request->validate([
            'operation_token' => ['required', 'string'],
        ]);

        $this->assertHighRiskToken($request, $validated['operation_token']);

        /** @var User $user */
        $user = $request->user();
        $user->forceFill([
            'two_factor_secret' => null,
            'two_factor_confirmed_at' => null,
        ])->save();

        $request->session()->forget(['admin_two_factor_verified', 'two_factor_pending_secret']);

        AuditLog::create([
            'user_id' => $user->id,
            'device_id' => null,
            'event_type' => 'auth.admin_two_factor_disabled',
            'ip_address' => $request->ip(),
            'message' => 'Admin disabled two-factor authentication',
            'metadata' => [],
        ]);

        return redirect()
            ->route('admin.two-factor.setup')
            ->with('status', '两步验证已关闭。');
    }

    private function authorizeAdmin(Request $request): void
    {
        abort_unless($request->user()?->is_admin, 403, 'Admin access required');
    }

    private function assertHighRiskToken(Request $request, string $rawToken): void
    {
        $rawToken = trim($rawToken);
        if ($rawToken === '') {
            throw ValidationException::withMessages([
                'operation_token' => '需要填写一次性确认码。',
            ]);
        }

        $candidateTokens = AdminOperationToken::query()
            ->where('user_id', (int) $request->user()->id)
            ->where('purpose', 'high_risk_admin_action')
            ->whereNull('used_at')
            ->where('expires_at', '>', now())
            ->latest()
            ->limit(10)
            ->get();

        $matched = $candidateTokens->first(
            fn (AdminOperationToken $token): bool => Hash::check($rawToken, $token->token_hash)
        );

        if (! $matched) {
            throw ValidationException::withMessages([
                'operation_token' => '一次性确认码无效或已过期。',
            ]);
        }

        $matched->update(['used_at' => now()]);
    }
}
