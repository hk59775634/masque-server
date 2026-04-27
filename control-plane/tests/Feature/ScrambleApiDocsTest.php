<?php

namespace Tests\Feature;

use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class ScrambleApiDocsTest extends TestCase
{
    use RefreshDatabase;

    public function test_api_docs_ui_forbidden_for_guests(): void
    {
        $this->get('/docs/api')->assertForbidden();
    }

    public function test_api_docs_ui_forbidden_for_non_admin(): void
    {
        $user = User::factory()->create(['is_admin' => false]);

        $this->actingAs($user)->get('/docs/api')->assertForbidden();
    }

    public function test_api_docs_ui_ok_for_admin(): void
    {
        $user = User::factory()->create(['is_admin' => true]);

        $this->actingAs($user)->get('/docs/api')->assertOk();
    }

    public function test_openapi_json_forbidden_for_guests(): void
    {
        $this->get('/docs/api.json')->assertForbidden();
    }

    public function test_openapi_json_ok_for_admin(): void
    {
        $user = User::factory()->create(['is_admin' => true]);

        $response = $this->actingAs($user)->get('/docs/api.json');
        $response->assertOk();
        $this->assertArrayHasKey('openapi', $response->json());
    }
}
