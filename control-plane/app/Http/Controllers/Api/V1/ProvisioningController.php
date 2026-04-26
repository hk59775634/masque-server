<?php

namespace App\Http\Controllers\Api\V1;

use App\Models\ActivationCode;
use App\Models\AuditLog;
use App\Models\Device;
use App\Models\User;
use App\Http\Controllers\Controller;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\Hash;
use Illuminate\Support\Str;

class ProvisioningController extends Controller
{
    private const TOKEN_TTL_HOURS = 12;

    public function createUser(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'name' => ['required', 'string', 'max:255'],
            'email' => ['required', 'email', 'max:255', 'unique:users,email'],
            'password' => [
                'required',
                'string',
                'min:8',
            ],
        ]);

        $user = User::create($validated);
        $this->audit('user.created', "User {$user->email} created", $request, $user->id);

        return response()->json(['user' => $user], 201);
    }

    public function issueActivationCode(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'user_id' => ['required', 'exists:users,id'],
            'device_name' => ['required', 'string', 'max:100'],
            'fingerprint' => ['required', 'string', 'max:255'],
        ]);

        $rawCode = Str::upper(Str::random(4).'-'.Str::random(4));
        $activation = ActivationCode::create([
            'user_id' => $validated['user_id'],
            'device_name' => $validated['device_name'],
            'fingerprint' => $validated['fingerprint'],
            'code_hash' => Hash::make($rawCode),
            'expires_at' => now()->addMinutes(20),
        ]);

        $this->audit('device.activation_code_issued', 'Activation code issued', $request, $validated['user_id'], null, [
            'activation_code_id' => $activation->id,
            'device_name' => $validated['device_name'],
        ]);

        return response()->json([
            'activation_code' => $rawCode,
            'expires_at' => $activation->expires_at?->toIso8601String(),
        ], 201);
    }

    public function activateDevice(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'activation_code' => ['required', 'string'],
            'fingerprint' => ['required', 'string', 'max:255'],
        ]);

        $candidateCodes = ActivationCode::query()
            ->where('fingerprint', $validated['fingerprint'])
            ->whereNull('used_at')
            ->where('expires_at', '>', Carbon::now())
            ->orderByDesc('id')
            ->limit(10)
            ->get();

        $activation = $candidateCodes->first(
            fn (ActivationCode $item): bool => Hash::check($validated['activation_code'], $item->code_hash)
        );

        if (!$activation) {
            return response()->json(['message' => 'Invalid or expired activation code'], 422);
        }

        $tokenExpiresAt = now()->addHours(self::TOKEN_TTL_HOURS);
        $jwtToken = $this->issueDeviceJwt($activation->user_id, $activation->fingerprint, $tokenExpiresAt);
        $device = Device::create([
            'user_id' => $activation->user_id,
            'device_name' => $activation->device_name,
            'fingerprint' => $activation->fingerprint,
            'status' => 'active',
            'api_token_hash' => hash('sha256', $jwtToken),
            'policy_acl' => ['allow' => [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']]],
            'policy_routes' => ['mode' => 'all', 'preserve' => ['127.0.0.1/32']],
            'policy_dns' => ['servers' => ['1.1.1.1', '8.8.8.8']],
            'last_seen_at' => now(),
            'token_expires_at' => $tokenExpiresAt,
        ]);

        $activation->update(['used_at' => now()]);
        $this->audit('device.activated', 'Device activated', $request, $activation->user_id, $device->id);

        return response()->json([
            'device_id' => $device->id,
            'device_token' => $jwtToken,
            'config' => [
                'server_addr' => config('app.url', 'http://127.0.0.1:8443'),
                'sni' => 'masque.afbuyers.local',
                'alpn' => 'h3',
                'dns' => ['1.1.1.1', '8.8.8.8'],
                'routes' => ['0.0.0.0/1', '128.0.0.0/1'],
                'policy_version' => 1,
            ],
        ]);
    }

    public function fetchDeviceSelf(Request $request): JsonResponse
    {
        $device = $this->deviceFromBearer($request);
        if (!$device) {
            return response()->json(['message' => 'Unauthorized'], 401);
        }

        $device->update(['last_seen_at' => now()]);
        $policy = $this->resolvedPolicy($device);
        $this->audit('device.self_fetched', 'Device self profile fetched', $request, $device->user_id, $device->id);

        return response()->json([
            'device' => [
                'id' => $device->id,
                'user_id' => $device->user_id,
                'device_name' => $device->device_name,
                'fingerprint' => $device->fingerprint,
                'status' => $device->status,
                'last_seen_at' => $device->last_seen_at?->toIso8601String(),
                'token_expires_at' => $device->token_expires_at?->toIso8601String(),
            ],
            'policy' => [
                'acl' => $policy['acl'],
                'routes' => $policy['routes'],
                'route_mode' => $policy['route_mode'],
                'dns' => $policy['dns'],
            ],
        ]);
    }

    public function fetchConfig(Request $request): JsonResponse
    {
        $device = $this->deviceFromBearer($request);
        if (!$device) {
            return response()->json(['message' => 'Unauthorized'], 401);
        }

        $device->update(['last_seen_at' => now()]);
        $policy = $this->resolvedPolicy($device);
        $this->audit('device.config_fetched', 'Device config fetched', $request, $device->user_id, $device->id);

        return response()->json([
            'device' => [
                'id' => $device->id,
                'name' => $device->device_name,
                'status' => $device->status,
            ],
            'config' => [
                'server_addr' => config('app.url', 'http://127.0.0.1:8443'),
                'sni' => 'masque.afbuyers.local',
                'alpn' => 'h3',
                'dns' => $policy['dns'],
                'routes' => $policy['routes'],
                'acl' => $policy['acl'],
                'route_mode' => $policy['route_mode'],
                'policy_version' => 1,
            ],
        ]);
    }

    public function authorizeSession(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'device_token' => ['required', 'string'],
            'fingerprint' => ['required', 'string', 'max:255'],
        ]);

        $claims = $this->parseDeviceJwt($validated['device_token']);
        $device = Device::query()
            ->where('fingerprint', $validated['fingerprint'])
            ->where('api_token_hash', hash('sha256', $validated['device_token']))
            ->where('status', 'active')
            ->where(function ($query): void {
                $query->whereNull('token_expires_at')->orWhere('token_expires_at', '>', now());
            })
            ->first();

        if (!$claims || !$device || ($claims['fingerprint'] ?? '') !== $validated['fingerprint']) {
            $this->audit('server.auth_failed', 'Server authorization failed', $request, null, null, [
                'fingerprint' => $validated['fingerprint'],
            ]);
            return response()->json(['allowed' => false], 401);
        }

        $device->update(['last_seen_at' => now()]);
        $policy = $this->resolvedPolicy($device);
        $this->audit('server.auth_ok', 'Server authorization passed', $request, $device->user_id, $device->id);

        return response()->json([
            'allowed' => true,
            'device_id' => $device->id,
            'user_id' => $device->user_id,
            'acl' => $policy['acl'],
            'routes' => $policy['routes'],
            'dns' => $policy['dns'],
        ]);
    }

    public function setUserPolicy(Request $request, User $user): JsonResponse
    {
        $validated = $request->validate([
            'acl' => ['nullable', 'array'],
            'routes' => ['nullable', 'array'],
            'dns' => ['nullable', 'array'],
        ]);

        $user->update([
            'policy_acl' => $validated['acl'] ?? $user->policy_acl,
            'policy_routes' => $validated['routes'] ?? $user->policy_routes,
            'policy_dns' => $validated['dns'] ?? $user->policy_dns,
        ]);

        $this->audit('policy.user.updated', 'User policy updated', $request, $user->id);

        return response()->json(['ok' => true]);
    }

    public function setDevicePolicy(Request $request, Device $device): JsonResponse
    {
        $validated = $request->validate([
            'acl' => ['nullable', 'array'],
            'routes' => ['nullable', 'array'],
            'dns' => ['nullable', 'array'],
        ]);

        $device->update([
            'policy_acl' => $validated['acl'] ?? $device->policy_acl,
            'policy_routes' => $validated['routes'] ?? $device->policy_routes,
            'policy_dns' => $validated['dns'] ?? $device->policy_dns,
        ]);

        $this->audit('policy.device.updated', 'Device policy updated', $request, $device->user_id, $device->id);

        return response()->json(['ok' => true]);
    }

    private function deviceFromBearer(Request $request): ?Device
    {
        $bearer = (string) $request->bearerToken();
        if ($bearer === '') {
            return null;
        }

        return Device::query()
            ->where('api_token_hash', hash('sha256', $bearer))
            ->where('status', 'active')
            ->where(function ($query): void {
                $query->whereNull('token_expires_at')->orWhere('token_expires_at', '>', now());
            })
            ->first();
    }

    private function resolvedPolicy(Device $device): array
    {
        $user = $device->user;
        $acl = $device->policy_acl ?? $user?->policy_acl ?? ['allow' => [['cidr' => '0.0.0.0/0', 'protocol' => 'any', 'port' => 'any']]];
        $routes = $device->policy_routes ?? $user?->policy_routes ?? ['mode' => 'all', 'include' => ['0.0.0.0/1', '128.0.0.0/1']];
        $dns = $device->policy_dns ?? $user?->policy_dns ?? ['servers' => ['1.1.1.1', '8.8.8.8']];

        return [
            'acl' => $acl,
            'routes' => $routes['include'] ?? ['0.0.0.0/1', '128.0.0.0/1'],
            'route_mode' => $routes['mode'] ?? 'all',
            'dns' => $dns['servers'] ?? ['1.1.1.1', '8.8.8.8'],
        ];
    }

    private function issueDeviceJwt(int $userId, string $fingerprint, Carbon $exp): string
    {
        $payload = [
            'sub' => $userId,
            'fingerprint' => $fingerprint,
            'iat' => now()->timestamp,
            'exp' => $exp->timestamp,
            'jti' => (string) Str::uuid(),
        ];

        return $this->encodeJwt($payload);
    }

    private function encodeJwt(array $payload): string
    {
        $header = ['alg' => 'HS256', 'typ' => 'JWT'];
        $headerEncoded = $this->base64UrlEncode(json_encode($header, JSON_UNESCAPED_SLASHES) ?: '{}');
        $payloadEncoded = $this->base64UrlEncode(json_encode($payload, JSON_UNESCAPED_SLASHES) ?: '{}');
        $signature = hash_hmac('sha256', "{$headerEncoded}.{$payloadEncoded}", (string) config('app.key'), true);

        return "{$headerEncoded}.{$payloadEncoded}.{$this->base64UrlEncode($signature)}";
    }

    private function parseDeviceJwt(string $token): ?array
    {
        $parts = explode('.', $token);
        if (count($parts) !== 3) {
            return null;
        }

        [$h, $p, $s] = $parts;
        $expected = $this->base64UrlEncode(hash_hmac('sha256', "{$h}.{$p}", (string) config('app.key'), true));
        if (!hash_equals($expected, $s)) {
            return null;
        }

        $payload = json_decode($this->base64UrlDecode($p), true);
        if (!is_array($payload)) {
            return null;
        }

        if (($payload['exp'] ?? 0) < now()->timestamp) {
            return null;
        }

        return $payload;
    }

    private function base64UrlEncode(string $input): string
    {
        return rtrim(strtr(base64_encode($input), '+/', '-_'), '=');
    }

    private function base64UrlDecode(string $input): string
    {
        $padLength = 4 - (strlen($input) % 4);
        if ($padLength < 4) {
            $input .= str_repeat('=', $padLength);
        }

        return base64_decode(strtr($input, '-_', '+/')) ?: '';
    }

    private function audit(
        string $eventType,
        string $message,
        Request $request,
        ?int $userId = null,
        ?int $deviceId = null,
        ?array $metadata = null
    ): void {
        AuditLog::create([
            'user_id' => $userId,
            'device_id' => $deviceId,
            'event_type' => $eventType,
            'ip_address' => $request->ip(),
            'message' => $message,
            'metadata' => $metadata,
        ]);
    }
}
