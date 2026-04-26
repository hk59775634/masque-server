<?php

namespace App\Http\Middleware;

use App\Models\ApiIdempotencyKey;
use Closure;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Database\QueryException;
use Symfony\Component\HttpFoundation\Response;

class EnsureApiIdempotency
{
    public function handle(Request $request, Closure $next): Response
    {
        $idempotencyKey = trim((string) $request->header('Idempotency-Key', ''));
        if ($idempotencyKey === '') {
            return $next($request);
        }

        $method = strtoupper($request->method());
        if (!in_array($method, ['POST', 'PUT', 'PATCH', 'DELETE'], true)) {
            return $next($request);
        }

        $path = (string) $request->path();
        $requestHash = hash('sha256', (string) $request->getContent());

        $record = ApiIdempotencyKey::query()
            ->where('idempotency_key', $idempotencyKey)
            ->where('method', $method)
            ->where('path', $path)
            ->first();

        if ($record) {
            if ($record->request_hash !== $requestHash) {
                return new JsonResponse([
                    'message' => 'Idempotency-Key reuse with different payload is not allowed',
                ], 409);
            }

            if ($record->finished_at && $record->response_body !== null && $record->status_code !== null) {
                $payload = json_decode($record->response_body, true);
                $response = new JsonResponse(
                    is_array($payload) ? $payload : ['message' => 'Replay response unavailable'],
                    (int) $record->status_code
                );
                $response->headers->set('X-Idempotent-Replay', 'true');

                return $response;
            }

            return new JsonResponse(['message' => 'Request with this Idempotency-Key is still processing'], 409);
        }

        try {
            $record = ApiIdempotencyKey::create([
                'idempotency_key' => $idempotencyKey,
                'method' => $method,
                'path' => $path,
                'request_hash' => $requestHash,
            ]);
        } catch (QueryException $e) {
            // Another concurrent request inserted the same unique key first.
            $record = ApiIdempotencyKey::query()
                ->where('idempotency_key', $idempotencyKey)
                ->where('method', $method)
                ->where('path', $path)
                ->first();
            if (!$record) {
                throw $e;
            }

            if ($record->request_hash !== $requestHash) {
                return new JsonResponse([
                    'message' => 'Idempotency-Key reuse with different payload is not allowed',
                ], 409);
            }

            if ($record->finished_at && $record->response_body !== null && $record->status_code !== null) {
                $payload = json_decode($record->response_body, true);
                $response = new JsonResponse(
                    is_array($payload) ? $payload : ['message' => 'Replay response unavailable'],
                    (int) $record->status_code
                );
                $response->headers->set('X-Idempotent-Replay', 'true');
                return $response;
            }

            return new JsonResponse(['message' => 'Request with this Idempotency-Key is still processing'], 409);
        }

        $response = $next($request);

        if ($response instanceof JsonResponse) {
            $rawContent = $response->getContent();
            if (is_string($rawContent)) {
                $record->update([
                    'status_code' => $response->getStatusCode(),
                    'response_body' => $rawContent,
                    'finished_at' => now(),
                ]);
            }
        }

        return $response;
    }
}
