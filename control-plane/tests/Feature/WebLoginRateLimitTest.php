<?php

namespace Tests\Feature;

use App\Models\AuditLog;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class WebLoginRateLimitTest extends TestCase
{
    use RefreshDatabase;

    protected function setUp(): void
    {
        parent::setUp();
        config(['security.web_login.max_attempts' => 3]);
        config(['security.web_login.decay_minutes' => 1]);
    }

    public function test_failed_logins_are_audited(): void
    {
        User::factory()->create(['email' => 'u@example.com']);

        $this->post(route('login.store'), [
            'email' => 'u@example.com',
            'password' => 'wrong',
        ])->assertSessionHasErrors('email');

        $this->post(route('login.store'), [
            'email' => 'u@example.com',
            'password' => 'wrong',
        ])->assertSessionHasErrors('email');

        $this->assertSame(2, AuditLog::query()->where('event_type', 'auth.web_login_failed')->count());
    }

    public function test_lockout_after_max_failed_attempts(): void
    {
        User::factory()->create(['email' => 'lock@example.com']);

        for ($i = 0; $i < 3; $i++) {
            $this->post(route('login.store'), [
                'email' => 'lock@example.com',
                'password' => 'wrong',
            ]);
        }

        $response = $this->post(route('login.store'), [
            'email' => 'lock@example.com',
            'password' => 'wrong',
        ]);

        $response->assertSessionHasErrors('email');
        $this->assertStringContainsString(
            '登录尝试次数过多',
            (string) session('errors')->get('email')[0]
        );

        $this->assertSame(3, AuditLog::query()->where('event_type', 'auth.web_login_failed')->count());
    }

    public function test_success_after_decay_and_audits_success(): void
    {
        User::factory()->create(['email' => 'ok@example.com']);

        for ($i = 0; $i < 3; $i++) {
            $this->post(route('login.store'), [
                'email' => 'ok@example.com',
                'password' => 'wrong',
            ]);
        }

        $this->travel(61)->minutes();

        $this->post(route('login.store'), [
            'email' => 'ok@example.com',
            'password' => 'password',
        ])->assertRedirect(route('dashboard'));

        $this->assertSame(1, AuditLog::query()->where('event_type', 'auth.web_login_success')->count());
    }

    public function test_success_clears_rate_limiter_for_next_session(): void
    {
        User::factory()->create(['email' => 'clear@example.com']);

        $this->post(route('login.store'), [
            'email' => 'clear@example.com',
            'password' => 'wrong',
        ]);

        $this->post(route('login.store'), [
            'email' => 'clear@example.com',
            'password' => 'password',
        ])->assertRedirect(route('dashboard'));

        $this->post(route('logout'), []);

        $this->post(route('login.store'), [
            'email' => 'clear@example.com',
            'password' => 'wrong',
        ])->assertSessionHasErrors('email');

        $this->assertStringContainsString(
            '邮箱或密码错误',
            (string) session('errors')->get('email')[0]
        );
    }
}
