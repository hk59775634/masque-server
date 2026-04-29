<?php

namespace App\Console\Commands;

use App\Models\Role;
use App\Models\User;
use Illuminate\Console\Command;

class PromoteUserAdminCommand extends Command
{
    protected $signature = 'user:promote-admin {id=1 :User primary key}';

    protected $description = 'Set is_admin=true for the given user id (bootstrap / recovery).';

    public function handle(): int
    {
        $id = (int) $this->argument('id');
        $user = User::query()->find($id);
        if (! $user) {
            $this->error("User id={$id} not found.");

            return self::FAILURE;
        }

        $user->is_admin = true;
        $user->save();
        $adminRoleId = Role::query()->where('name', 'admin')->value('id');
        if ($adminRoleId !== null) {
            $user->roles()->syncWithoutDetaching([(int) $adminRoleId]);
        }

        $this->info("User {$user->email} (id={$user->id}) is now admin.");

        return self::SUCCESS;
    }
}
