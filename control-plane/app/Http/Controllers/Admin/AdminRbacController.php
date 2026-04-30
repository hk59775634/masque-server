<?php

namespace App\Http\Controllers\Admin;

use App\Http\Controllers\Controller;
use App\Http\Controllers\Admin\Concerns\ConfirmsHighRiskAdminOperations;
use App\Models\Permission;
use App\Models\Role;
use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Validation\ValidationException;
use Illuminate\View\View;

class AdminRbacController extends Controller
{
    use ConfirmsHighRiskAdminOperations;

    public function index(Request $request): View
    {
        $this->authorizeRbac($request);
        $roles = Role::query()->with('permissions')->orderBy('name')->get();
        $permissions = Permission::query()->orderBy('name')->get();
        $users = User::query()->orderBy('id')->limit(500)->get(['id', 'email', 'name']);

        return view('admin.rbac.index', [
            'roles' => $roles,
            'permissions' => $permissions,
            'users' => $users,
        ]);
    }

    public function store(Request $request): RedirectResponse
    {
        $this->authorizeRbac($request);
        $data = $request->validate([
            'name' => ['required', 'string', 'regex:/^[a-z0-9][a-z0-9_-]*$/', 'max:64', 'unique:roles,name'],
            'display_name' => ['nullable', 'string', 'max:120'],
        ]);
        if ($data['name'] === 'admin') {
            throw ValidationException::withMessages(['name' => '保留角色名 admin 不可新建。']);
        }

        $role = Role::query()->create([
            'name' => $data['name'],
            'display_name' => $data['display_name'] ?? null,
        ]);
        $this->writeAudit($request, 'admin.rbac_role_created', "Created role: {$role->name}", null, null, [
            'role_id' => $role->id,
            'name' => $role->name,
        ]);

        return back()->with('status', '角色已创建。');
    }

    public function updateRolePermissions(Request $request, Role $role): RedirectResponse
    {
        $this->authorizeRbac($request);
        $payload = $request->validate([
            'permission_ids' => ['required', 'array'],
            'permission_ids.*' => ['integer', 'exists:permissions,id'],
            'operation_token' => ['nullable', 'string', 'max:32'],
        ]);

        $newIds = array_values(array_unique(array_map(intval(...), $payload['permission_ids'])));
        sort($newIds);
        $oldIds = $role->permissions()->pluck('permissions.id')->map(fn ($v): int => (int) $v)->sort()->values()->all();
        $idsEqual = $newIds === $oldIds;

        $rbacWriteId = (int) Permission::query()->where('name', 'admin.rbac.write')->value('id');
        $isGrantingRbacWrite = $rbacWriteId > 0
            && in_array($rbacWriteId, $newIds, true)
            && ! in_array($rbacWriteId, $oldIds, true);
        $isHighRisk = (! $idsEqual && $role->name === 'admin') || $isGrantingRbacWrite;

        $this->assertHighRiskConfirmed($request, $isHighRisk, $payload['operation_token'] ?? null);

        if ($role->name === 'admin') {
            $accessId = (int) Permission::query()->where('name', 'admin.access')->value('id');
            if ($accessId > 0 && ! in_array($accessId, $newIds, true)) {
                throw ValidationException::withMessages(['permission_ids' => 'admin 角色必须保留「访问管理台」权限 (admin.access)。']);
            }
        }

        $role->permissions()->sync($newIds);
        $this->writeAudit($request, 'admin.rbac_role_permissions_updated', "Updated permissions for role: {$role->name}", null, null, [
            'role_id' => $role->id,
            'role_name' => $role->name,
            'before_permission_ids' => $oldIds,
            'after_permission_ids' => $newIds,
        ]);

        return back()->with('status', "角色 {$role->name} 的权限已更新。");
    }

    public function syncUserRoles(Request $request): RedirectResponse
    {
        $this->authorizeRbac($request);
        $payload = $request->validate([
            'user_id' => ['required', 'integer', 'exists:users,id'],
            'role_ids' => ['nullable', 'array'],
            'role_ids.*' => ['integer', 'exists:roles,id'],
            'is_admin_mode' => ['required', 'in:keep,0,1'],
            'operation_token' => ['nullable', 'string', 'max:32'],
        ]);

        $user = User::query()->findOrFail($payload['user_id']);
        $afterIsAdmin = match ($payload['is_admin_mode']) {
            '1' => true,
            '0' => false,
            default => (bool) $user->is_admin,
        };
        $adminRoleId = Role::query()->where('name', 'admin')->value('id');
        $requestedRoleIds = collect($payload['role_ids'] ?? [])->map(fn ($id): int => (int) $id)->unique()->values();
        if ($adminRoleId !== null && $afterIsAdmin && ! $requestedRoleIds->contains((int) $adminRoleId)) {
            $requestedRoleIds->push((int) $adminRoleId);
        }
        $roleNamesByRequest = Role::query()->whereIn('id', $requestedRoleIds->all())->pluck('name')->values()->all();
        $willBeAdmin = $afterIsAdmin || in_array('admin', $roleNamesByRequest, true);
        $isHighRisk = ($user->is_admin !== $willBeAdmin);
        $this->assertHighRiskConfirmed($request, $isHighRisk, $payload['operation_token'] ?? null);

        $beforeRoleNames = $user->roles()->pluck('name')->values()->all();
        $beforeIsAdmin = (bool) $user->is_admin;
        $user->roles()->sync($requestedRoleIds->all());
        $afterRoleNames = $user->roles()->pluck('name')->values()->all();
        $adminByRole = in_array('admin', $afterRoleNames, true);
        $user->update([
            'is_admin' => $afterIsAdmin || $adminByRole,
        ]);
        $user->refresh();

        $this->writeAudit($request, 'admin.rbac_user_roles_synced', "RBAC 页同步用户角色: {$user->email}", $user->id, null, [
            'before' => ['roles' => $beforeRoleNames, 'is_admin' => $beforeIsAdmin],
            'after' => ['roles' => $afterRoleNames, 'is_admin' => (bool) $user->is_admin],
        ]);

        return back()->with('status', '用户角色已同步。');
    }

    protected function authorizeRbac(Request $request): void
    {
        abort_unless($request->user()?->hasPermission('admin.rbac.write'), 403, 'RBAC 管理需要 admin.rbac.write 权限');
    }
}
