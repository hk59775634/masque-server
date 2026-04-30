<?php

namespace Tests\Feature;

use App\Models\AdminOperationToken;
use App\Models\Permission;
use App\Models\Role;
use App\Models\User;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Hash;
use Tests\TestCase;

class AdminRbacTest extends TestCase
{
    use RefreshDatabase;

    public function test_user_with_admin_role_can_access_admin_console_without_is_admin_flag(): void
    {
        $user = User::factory()->create(['is_admin' => false]);
        $adminRole = Role::query()->where('name', 'admin')->firstOrFail();
        $user->roles()->syncWithoutDetaching([$adminRole->id]);

        $this->actingAs($user)
            ->get(route('admin.index', ['tab' => 'overview']))
            ->assertOk();
    }

    public function test_assigning_admin_role_requires_operation_token(): void
    {
        $operator = User::factory()->create(['is_admin' => true]);
        $target = User::factory()->create(['is_admin' => false]);
        $adminRole = Role::query()->where('name', 'admin')->firstOrFail();

        // Missing one-time confirmation token should fail (high-risk admin privilege grant).
        $this->actingAs($operator)
            ->post(route('admin.users.policy', $target), [
                'role_ids' => [(string) $adminRole->id],
                'route_mode' => 'all',
                'routes' => ['0.0.0.0/1', '128.0.0.0/1'],
                'dns_servers' => ['1.1.1.1'],
                'acl_rules' => [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']],
            ])
            ->assertSessionHasErrors('operation_token');

        $plainToken = 'ABC-DEF';
        AdminOperationToken::query()->create([
            'user_id' => $operator->id,
            'purpose' => 'high_risk_admin_action',
            'token_hash' => Hash::make($plainToken),
            'expires_at' => now()->addMinutes(5),
        ]);

        $this->actingAs($operator)
            ->post(route('admin.users.policy', $target), [
                'operation_token' => $plainToken,
                'role_ids' => [(string) $adminRole->id],
                'route_mode' => 'all',
                'routes' => ['0.0.0.0/1', '128.0.0.0/1'],
                'dns_servers' => ['1.1.1.1'],
                'acl_rules' => [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']],
            ])
            ->assertRedirect();

        $target->refresh();
        $this->assertTrue($target->is_admin);
        $this->assertTrue($target->roles()->where('name', 'admin')->exists());
    }

    public function test_auditor_role_cannot_open_rbac_management(): void
    {
        $user = User::factory()->create(['is_admin' => false]);
        $auditor = Role::query()->where('name', 'auditor')->firstOrFail();
        $user->roles()->sync([$auditor->id]);

        $this->actingAs($user)
            ->get(route('admin.rbac.index'))
            ->assertForbidden();
    }

    public function test_admin_role_user_can_open_rbac_management(): void
    {
        $user = User::factory()->create(['is_admin' => false]);
        $adminRole = Role::query()->where('name', 'admin')->firstOrFail();
        $user->roles()->sync([$adminRole->id]);

        $this->actingAs($user)
            ->get(route('admin.rbac.index'))
            ->assertOk();
    }

    public function test_updating_admin_role_permissions_requires_operation_token_when_changed(): void
    {
        $operator = User::factory()->create(['is_admin' => true]);
        $adminRole = Role::query()->where('name', 'admin')->firstOrFail();
        $permIds = Permission::query()->orderBy('id')->pluck('id')->map(fn ($v): int => (int) $v)->values()->all();
        $dropOne = array_values(array_filter($permIds, fn (int $id): bool => $id !== (int) Permission::query()->where('name', 'admin.access')->value('id')));

        $this->actingAs($operator)
            ->post(route('admin.rbac.roles.permissions', $adminRole), [
                'permission_ids' => $dropOne,
            ])
            ->assertSessionHasErrors('operation_token');
    }

    public function test_can_create_custom_role_via_rbac_page(): void
    {
        $operator = User::factory()->create(['is_admin' => true]);

        $this->actingAs($operator)
            ->post(route('admin.rbac.roles.store'), [
                'name' => 'custom_support',
                'display_name' => 'Support',
            ])
            ->assertRedirect();

        $this->assertDatabaseHas('roles', ['name' => 'custom_support']);
    }
}

