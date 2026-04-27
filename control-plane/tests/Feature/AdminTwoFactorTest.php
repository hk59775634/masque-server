<?php

namespace Tests\Feature;

use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use PragmaRX\Google2FA\Google2FA;
use Tests\TestCase;

class AdminTwoFactorTest extends TestCase
{
    use RefreshDatabase;

    public function test_admin_with_two_factor_is_redirected_to_challenge(): void
    {
        $secret = (new Google2FA)->generateSecretKey();
        $user = User::factory()->create(['is_admin' => true]);
        $user->two_factor_secret = $secret;
        $user->two_factor_confirmed_at = now();
        $user->save();

        $this->actingAs($user)
            ->get(route('admin.index', ['tab' => 'overview']))
            ->assertRedirect(route('admin.two-factor.challenge'));
    }

    public function test_admin_can_complete_challenge_and_reach_admin(): void
    {
        $google2fa = new Google2FA;
        $secret = $google2fa->generateSecretKey();
        $user = User::factory()->create(['is_admin' => true]);
        $user->two_factor_secret = $secret;
        $user->two_factor_confirmed_at = now();
        $user->save();

        $code = $google2fa->getCurrentOtp($secret);

        $this->actingAs($user)
            ->post(route('admin.two-factor.verify'), ['code' => $code])
            ->assertRedirect();

        $this->actingAs($user)
            ->get(route('admin.index', ['tab' => 'overview']))
            ->assertOk();
    }

    public function test_promote_admin_command(): void
    {
        $user = User::factory()->create(['is_admin' => false]);

        $this->artisan('user:promote-admin', ['id' => (string) $user->id])
            ->assertSuccessful();

        $this->assertTrue($user->fresh()->is_admin);
    }
}
