<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('roles', function (Blueprint $table): void {
            $table->id();
            $table->string('name')->unique();
            $table->string('display_name')->nullable();
            $table->timestamps();
        });

        Schema::create('permissions', function (Blueprint $table): void {
            $table->id();
            $table->string('name')->unique();
            $table->string('display_name')->nullable();
            $table->timestamps();
        });

        Schema::create('role_user', function (Blueprint $table): void {
            $table->id();
            $table->foreignId('role_id')->constrained()->cascadeOnDelete();
            $table->foreignId('user_id')->constrained()->cascadeOnDelete();
            $table->timestamps();
            $table->unique(['role_id', 'user_id']);
        });

        Schema::create('permission_role', function (Blueprint $table): void {
            $table->id();
            $table->foreignId('permission_id')->constrained()->cascadeOnDelete();
            $table->foreignId('role_id')->constrained()->cascadeOnDelete();
            $table->timestamps();
            $table->unique(['permission_id', 'role_id']);
        });

        // Minimal bootstrap role/permissions for current admin console.
        DB::table('roles')->insert([
            'name' => 'admin',
            'display_name' => 'Administrator',
            'created_at' => now(),
            'updated_at' => now(),
        ]);
        DB::table('permissions')->insert([
            [
                'name' => 'admin.access',
                'display_name' => 'Access admin console',
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'name' => 'admin.policy.write',
                'display_name' => 'Update user/device policy',
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'name' => 'admin.audit.read',
                'display_name' => 'Read/export audits',
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'name' => 'admin.session.revoke',
                'display_name' => 'Force logout users',
                'created_at' => now(),
                'updated_at' => now(),
            ],
        ]);

        $adminRoleId = (int) DB::table('roles')->where('name', 'admin')->value('id');
        $permissionIds = DB::table('permissions')->pluck('id')->map(fn ($v): int => (int) $v);
        foreach ($permissionIds as $permissionId) {
            DB::table('permission_role')->insert([
                'permission_id' => $permissionId,
                'role_id' => $adminRoleId,
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }

        // Backfill existing is_admin users into admin role.
        $adminUserIds = DB::table('users')->where('is_admin', true)->pluck('id')->map(fn ($v): int => (int) $v);
        foreach ($adminUserIds as $userId) {
            DB::table('role_user')->updateOrInsert(
                ['role_id' => $adminRoleId, 'user_id' => $userId],
                ['updated_at' => now(), 'created_at' => now()]
            );
        }
    }

    public function down(): void
    {
        Schema::dropIfExists('permission_role');
        Schema::dropIfExists('role_user');
        Schema::dropIfExists('permissions');
        Schema::dropIfExists('roles');
    }
};

