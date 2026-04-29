<?php

namespace Tests\Feature;

use App\Models\Device;
use App\Models\User;
use Illuminate\Support\Str;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class ApiDeviceSelfTest extends TestCase
{
    use RefreshDatabase;

    public function test_devices_self_requires_bearer(): void
    {
        $this->getJson('/api/v1/devices/self')
            ->assertStatus(401)
            ->assertJsonPath('message', 'Unauthorized');
    }

    public function test_devices_self_returns_profile_with_valid_token(): void
    {
        $this->postJson('/api/v1/users', [
            'name' => 'Self API User',
            'email' => 'selfapi@example.com',
            'password' => 'password123',
        ])->assertStatus(201);

        $user = User::query()->where('email', 'selfapi@example.com')->firstOrFail();

        $codeResp = $this->postJson('/api/v1/devices/activation-code', [
            'user_id' => $user->id,
            'device_name' => 'cli-test',
            'fingerprint' => 'fp-self-api',
        ]);
        $codeResp->assertStatus(201);
        $rawCode = $codeResp->json('activation_code');
        $this->assertNotEmpty($rawCode);

        $act = $this->postJson('/api/v1/activate', [
            'activation_code' => $rawCode,
            'fingerprint' => 'fp-self-api',
        ]);
        $act->assertStatus(200);
        $token = $act->json('device_token');
        $this->assertNotEmpty($token);

        $self = $this->withToken($token)->getJson('/api/v1/devices/self');
        $self->assertOk()
            ->assertJsonPath('device.device_name', 'cli-test')
            ->assertJsonPath('device.fingerprint', 'fp-self-api')
            ->assertJsonPath('device.status', 'active')
            ->assertJsonPath('policy.route_mode', 'all');

        $this->assertArrayNotHasKey('api_token_hash', $self->json('device') ?? []);
    }

    public function test_activate_reissues_token_when_fingerprint_already_registered_same_user(): void
    {
        $this->postJson('/api/v1/users', [
            'name' => 'Reactivate User',
            'email' => 'reactivate@example.com',
            'password' => 'password123',
        ])->assertStatus(201);

        $c1 = $this->postJson('/api/v1/devices/activation-code-with-credentials', [
            'email' => 'reactivate@example.com',
            'password' => 'password123',
            'fingerprint' => 'fp-reactivate-same',
            'device_name' => 'd1',
        ])->assertStatus(201)->json('activation_code');

        $a1 = $this->postJson('/api/v1/activate', [
            'activation_code' => $c1,
            'fingerprint' => 'fp-reactivate-same',
        ])->assertOk();
        $token1 = $a1->json('device_token');
        $deviceId = $a1->json('device_id');
        $this->assertNotEmpty($token1);

        $c2 = $this->postJson('/api/v1/devices/activation-code-with-credentials', [
            'email' => 'reactivate@example.com',
            'password' => 'password123',
            'fingerprint' => 'fp-reactivate-same',
            'device_name' => 'd2',
        ])->assertStatus(201)->json('activation_code');

        $a2 = $this->postJson('/api/v1/activate', [
            'activation_code' => $c2,
            'fingerprint' => 'fp-reactivate-same',
        ])->assertOk();
        $token2 = $a2->json('device_token');
        $this->assertNotEmpty($token2);
        $this->assertNotSame($token1, $token2);
        $this->assertSame($deviceId, $a2->json('device_id'));

        $this->assertSame(1, Device::query()->where('fingerprint', 'fp-reactivate-same')->count());
    }

    public function test_issue_activation_code_with_credentials_requires_valid_login(): void
    {
        $this->postJson('/api/v1/users', [
            'name' => 'Cred User',
            'email' => 'creduser@example.com',
            'password' => 'password123',
        ])->assertStatus(201);

        $this->postJson('/api/v1/devices/activation-code-with-credentials', [
            'email' => 'creduser@example.com',
            'password' => 'wrong-password',
            'fingerprint' => 'fp-cred-bootstrap',
            'device_name' => 'cli',
        ])->assertStatus(401);

        $resp = $this->postJson('/api/v1/devices/activation-code-with-credentials', [
            'email' => 'creduser@example.com',
            'password' => 'password123',
            'fingerprint' => 'fp-cred-bootstrap',
            'device_name' => 'cli',
        ]);
        $resp->assertStatus(201)
            ->assertJsonPath('fingerprint', 'fp-cred-bootstrap');
        $this->assertNotEmpty($resp->json('activation_code'));

        $this->postJson('/api/v1/activate', [
            'activation_code' => $resp->json('activation_code'),
            'fingerprint' => 'fp-cred-bootstrap',
        ])->assertOk();
    }

    public function test_activate_returns_masque_server_url_from_services_config(): void
    {
        config(['services.masque.server_url' => 'http://masque.test:9443']);

        $this->postJson('/api/v1/users', [
            'name' => 'Masque URL User',
            'email' => 'masqueurl@example.com',
            'password' => 'password123',
        ])->assertStatus(201);

        $user = User::query()->where('email', 'masqueurl@example.com')->firstOrFail();

        $codeResp = $this->postJson('/api/v1/devices/activation-code', [
            'user_id' => $user->id,
            'device_name' => 'm-url',
            'fingerprint' => 'fp-masque-url',
        ]);
        $codeResp->assertStatus(201);
        $rawCode = $codeResp->json('activation_code');

        $act = $this->postJson('/api/v1/activate', [
            'activation_code' => $rawCode,
            'fingerprint' => 'fp-masque-url',
        ]);
        $act->assertOk()
            ->assertJsonPath('config.server_addr', 'http://masque.test:9443');
    }

    public function test_device_token_rotate_and_revoke(): void
    {
        [$token, $fingerprint] = $this->activateDemoDevice('token-flow@example.com', 'fp-token-flow');

        $rotate = $this->withToken($token)->postJson('/api/v1/device/token/rotate', [
            'fingerprint' => $fingerprint,
        ]);
        $rotate->assertOk();
        $newToken = (string) $rotate->json('device_token');
        $this->assertNotSame($token, $newToken);

        // Old token should be invalid immediately after rotation.
        $this->withToken($token)->getJson('/api/v1/devices/self')
            ->assertStatus(401);

        // New token should work.
        $this->withToken($newToken)->getJson('/api/v1/devices/self')
            ->assertOk()
            ->assertJsonPath('device.fingerprint', $fingerprint);

        $this->withToken($newToken)->postJson('/api/v1/device/token/revoke')
            ->assertOk()
            ->assertJsonPath('ok', true);

        // Revoked token should no longer work.
        $this->withToken($newToken)->getJson('/api/v1/devices/self')
            ->assertStatus(401);
    }

    public function test_server_authorize_hmac_required_rejects_unsigned_and_accepts_signed(): void
    {
        [$token, $fingerprint] = $this->activateDemoDevice('hmac-authz@example.com', 'fp-hmac-authz');

        config([
            'services.masque.authorize_hmac_secret' => 'test-hmac-secret',
            'services.masque.authorize_hmac_required' => true,
            'services.masque.authorize_hmac_window_seconds' => 300,
        ]);

        $payload = [
            'device_token' => $token,
            'fingerprint' => $fingerprint,
        ];

        $this->postJson('/api/v1/server/authorize', $payload)
            ->assertStatus(401)
            ->assertJsonPath('error', 'missing_signature_headers');

        $body = json_encode($payload, JSON_UNESCAPED_SLASHES);
        $this->assertIsString($body);
        $ts = (string) now()->timestamp;
        $macPayload = implode("\n", [
            'POST',
            '/api/v1/server/authorize',
            $ts,
            hash('sha256', $body),
        ]);
        $sig = hash_hmac('sha256', $macPayload, 'test-hmac-secret');

        $this->withHeaders([
            'X-Masque-Authz-Timestamp' => $ts,
            'X-Masque-Authz-Signature' => $sig,
        ])->postJson('/api/v1/server/authorize', $payload)
            ->assertOk()
            ->assertJsonPath('allowed', true)
            ->assertJsonPath('device_id', Device::query()->where('fingerprint', $fingerprint)->firstOrFail()->id);
    }

    /**
     * @return array{0:string,1:string}
     */
    private function activateDemoDevice(string $email, string $fingerprint): array
    {
        $this->postJson('/api/v1/users', [
            'name' => 'Demo '.Str::before($email, '@'),
            'email' => $email,
            'password' => 'password123',
        ])->assertStatus(201);

        $user = User::query()->where('email', $email)->firstOrFail();
        $code = $this->postJson('/api/v1/devices/activation-code', [
            'user_id' => $user->id,
            'device_name' => 'demo-device',
            'fingerprint' => $fingerprint,
        ])->assertStatus(201)->json('activation_code');

        $token = $this->postJson('/api/v1/activate', [
            'activation_code' => $code,
            'fingerprint' => $fingerprint,
        ])->assertOk()->json('device_token');

        return [(string) $token, $fingerprint];
    }
}
