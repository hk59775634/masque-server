<?php

namespace App\Services;

use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;
use Throwable;

final class MasquePrometheusMetrics
{
    /**
     * @return array{reachable: bool, error: ?string, metrics: array<string, float|int|null>}
     */
    public function snapshot(): array
    {
        $base = rtrim((string) config('services.prometheus.url', ''), '/');
        if ($base === '') {
            return $this->failure('未配置 PROMETHEUS_URL');
        }

        $queries = [
            'connect_requests' => 'masque_connect_requests_total',
            'connect_success' => 'masque_connect_success_total',
            'connect_failures_sum' => 'sum(masque_connect_failures_total)',
            'healthz_requests' => 'masque_healthz_requests_total',
            'target_up' => 'up{job="masque-server"}',
        ];

        $metrics = [];
        try {
            foreach ($queries as $key => $promql) {
                $metrics[$key] = $this->instantScalar($base, $promql);
            }
        } catch (Throwable $e) {
            Log::warning('prometheus.snapshot_failed', ['message' => $e->getMessage()]);

            return $this->failure('查询 Prometheus 失败');
        }

        return [
            'reachable' => true,
            'error' => null,
            'metrics' => $metrics,
        ];
    }

    /**
     * @return array{reachable: bool, error: ?string, metrics: array<string, float|int|null>}
     */
    private function failure(string $message): array
    {
        return [
            'reachable' => false,
            'error' => $message,
            'metrics' => [
                'connect_requests' => null,
                'connect_success' => null,
                'connect_failures_sum' => null,
                'healthz_requests' => null,
                'target_up' => null,
            ],
        ];
    }

    private function instantScalar(string $prometheusBase, string $promql): ?float
    {
        $response = Http::timeout(2)
            ->acceptJson()
            ->get($prometheusBase.'/api/v1/query', ['query' => $promql]);

        if (! $response->successful()) {
            return null;
        }

        $data = $response->json('data');
        if (! is_array($data) || ($data['resultType'] ?? '') !== 'vector') {
            return null;
        }

        $result = $data['result'][0] ?? null;
        if (! is_array($result)) {
            return null;
        }

        $value = $result['value'][1] ?? null;
        if ($value === null || $value === 'NaN') {
            return null;
        }

        return is_numeric($value) ? (float) $value : null;
    }
}
