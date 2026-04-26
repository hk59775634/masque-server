<?php

namespace Tests\Feature;

use Tests\TestCase;

class ApiV1HealthTest extends TestCase
{
    public function test_health_returns_json_ok(): void
    {
        $this->getJson('/api/v1/health')
            ->assertOk()
            ->assertJson([
                'status' => 'ok',
                'service' => 'control-plane',
            ]);
    }
}
