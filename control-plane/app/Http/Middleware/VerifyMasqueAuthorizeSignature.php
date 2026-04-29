<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

class VerifyMasqueAuthorizeSignature
{
    private const HEADER_TS = 'X-Masque-Authz-Timestamp';
    private const HEADER_SIG = 'X-Masque-Authz-Signature';

    public function handle(Request $request, Closure $next): mixed
    {
        $secret = trim((string) config('services.masque.authorize_hmac_secret', ''));
        $required = (bool) config('services.masque.authorize_hmac_required', false);

        $ts = trim((string) $request->header(self::HEADER_TS, ''));
        $sig = strtolower(trim((string) $request->header(self::HEADER_SIG, '')));

        // Backward-compatible mode: if not required and caller did not sign, allow.
        if (! $required && $secret === '' && $ts === '' && $sig === '') {
            return $next($request);
        }
        if (! $required && ($ts === '' || $sig === '')) {
            return $next($request);
        }
        if ($secret === '') {
            return $this->deny('authorize_hmac_secret_not_configured');
        }
        if ($ts === '' || $sig === '') {
            return $this->deny('missing_signature_headers');
        }
        if (! ctype_digit($ts)) {
            return $this->deny('invalid_timestamp');
        }

        $windowSeconds = max(1, (int) config('services.masque.authorize_hmac_window_seconds', 300));
        $now = time();
        $tsInt = (int) $ts;
        if (abs($now - $tsInt) > $windowSeconds) {
            return $this->deny('signature_timestamp_out_of_window');
        }

        $payloadHash = hash('sha256', $request->getContent());
        $macPayload = implode("\n", [
            strtoupper($request->method()),
            '/api/v1/server/authorize',
            $ts,
            $payloadHash,
        ]);
        $expected = hash_hmac('sha256', $macPayload, $secret);
        if (! hash_equals($expected, $sig)) {
            return $this->deny('invalid_signature');
        }

        return $next($request);
    }

    private function deny(string $reason): JsonResponse
    {
        return response()->json([
            'allowed' => false,
            'error' => $reason,
        ], 401);
    }
}

