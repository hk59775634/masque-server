<?php

namespace Tests\Feature;

use App\Models\ApiIdempotencyKey;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class ApiIdempotencyMiddlewareTest extends TestCase
{
    use RefreshDatabase;

    public function test_replays_same_response_for_same_key_and_same_payload(): void
    {
        $payload = [
            'name' => 'Replay User',
            'email' => 'replay@example.com',
            'password' => 'password123',
        ];

        $first = $this->withHeaders(['Idempotency-Key' => 'idem-replay-1'])
            ->postJson('/api/v1/users', $payload);

        $first->assertStatus(201);

        $second = $this->withHeaders(['Idempotency-Key' => 'idem-replay-1'])
            ->postJson('/api/v1/users', $payload);

        $second->assertStatus(201)
            ->assertHeader('X-Idempotent-Replay', 'true')
            ->assertJsonPath('user.email', 'replay@example.com');

        $this->assertSame(1, User::query()->where('email', 'replay@example.com')->count());
    }

    public function test_returns_409_when_same_key_has_different_payload(): void
    {
        $firstPayload = [
            'name' => 'Conflict User',
            'email' => 'conflict-a@example.com',
            'password' => 'password123',
        ];
        $secondPayload = [
            'name' => 'Conflict User 2',
            'email' => 'conflict-b@example.com',
            'password' => 'password123',
        ];

        $this->withHeaders(['Idempotency-Key' => 'idem-conflict-1'])
            ->postJson('/api/v1/users', $firstPayload)
            ->assertStatus(201);

        $this->withHeaders(['Idempotency-Key' => 'idem-conflict-1'])
            ->postJson('/api/v1/users', $secondPayload)
            ->assertStatus(409)
            ->assertJsonPath('message', 'Idempotency-Key reuse with different payload is not allowed');
    }

    public function test_returns_409_when_same_key_is_still_processing(): void
    {
        $payload = [
            'name' => 'Pending User',
            'email' => 'pending@example.com',
            'password' => 'password123',
        ];

        ApiIdempotencyKey::create([
            'idempotency_key' => 'idem-pending-1',
            'method' => 'POST',
            'path' => 'api/v1/users',
            'request_hash' => hash('sha256', json_encode($payload) ?: ''),
            'status_code' => null,
            'response_body' => null,
            'finished_at' => null,
        ]);

        $this->withHeaders(['Idempotency-Key' => 'idem-pending-1'])
            ->postJson('/api/v1/users', $payload)
            ->assertStatus(409)
            ->assertJsonPath('message', 'Request with this Idempotency-Key is still processing');
    }
}
