<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    public function up(): void
    {
        if (! DB::table('permissions')->where('name', 'admin.rbac.write')->exists()) {
            DB::table('permissions')->insert([
                'name' => 'admin.rbac.write',
                'display_name' => 'Manage roles and permissions',
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }

        $rbacPermId = (int) DB::table('permissions')->where('name', 'admin.rbac.write')->value('id');
        $adminRoleId = DB::table('roles')->where('name', 'admin')->value('id');
        if ($adminRoleId !== null && $rbacPermId > 0) {
            DB::table('permission_role')->updateOrInsert(
                ['role_id' => (int) $adminRoleId, 'permission_id' => $rbacPermId],
                ['created_at' => now(), 'updated_at' => now()]
            );
        }

        $this->ensureTemplateRole('auditor', 'Auditor (read-only audits)', ['admin.access', 'admin.audit.read']);
        $this->ensureTemplateRole('ops', 'Operations', ['admin.access', 'admin.audit.read', 'admin.policy.write']);
        $this->ensureTemplateRole('security', 'Security / incident response', [
            'admin.access',
            'admin.audit.read',
            'admin.policy.write',
            'admin.session.revoke',
        ]);
    }

    /**
     * @param  list<string>  $permissionNames
     */
    private function ensureTemplateRole(string $name, string $displayName, array $permissionNames): void
    {
        DB::table('roles')->updateOrInsert(
            ['name' => $name],
            ['display_name' => $displayName, 'created_at' => now(), 'updated_at' => now()]
        );
        $roleId = (int) DB::table('roles')->where('name', $name)->value('id');
        $permissionIds = DB::table('permissions')->whereIn('name', $permissionNames)->pluck('id')->map(fn ($v): int => (int) $v);
        DB::table('permission_role')->where('role_id', $roleId)->delete();
        foreach ($permissionIds as $permissionId) {
            DB::table('permission_role')->insert([
                'role_id' => $roleId,
                'permission_id' => $permissionId,
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }
    }

    public function down(): void
    {
        foreach (['auditor', 'ops', 'security'] as $name) {
            $roleId = DB::table('roles')->where('name', $name)->value('id');
            if ($roleId === null) {
                continue;
            }
            DB::table('permission_role')->where('role_id', $roleId)->delete();
            DB::table('role_user')->where('role_id', $roleId)->delete();
            DB::table('roles')->where('id', $roleId)->delete();
        }

        $permId = DB::table('permissions')->where('name', 'admin.rbac.write')->value('id');
        if ($permId !== null) {
            DB::table('permission_role')->where('permission_id', $permId)->delete();
            DB::table('permissions')->where('id', $permId)->delete();
        }
    }
};
