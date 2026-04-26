<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        Schema::table('devices', function (Blueprint $table) {
            $table->json('policy_acl')->nullable()->after('api_token_hash');
            $table->json('policy_routes')->nullable()->after('policy_acl');
            $table->json('policy_dns')->nullable()->after('policy_routes');
            $table->timestamp('token_expires_at')->nullable()->after('last_seen_at');
            $table->index('token_expires_at');
        });

        Schema::table('users', function (Blueprint $table) {
            $table->json('policy_acl')->nullable()->after('remember_token');
            $table->json('policy_routes')->nullable()->after('policy_acl');
            $table->json('policy_dns')->nullable()->after('policy_routes');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::table('devices', function (Blueprint $table) {
            $table->dropIndex(['token_expires_at']);
            $table->dropColumn(['policy_acl', 'policy_routes', 'policy_dns', 'token_expires_at']);
        });

        Schema::table('users', function (Blueprint $table) {
            $table->dropColumn(['policy_acl', 'policy_routes', 'policy_dns']);
        });
    }
};
