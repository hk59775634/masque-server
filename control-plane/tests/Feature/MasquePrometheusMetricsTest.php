<?php

namespace Tests\Feature;

use App\Services\MasquePrometheusMetrics;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Config;
use Illuminate\Support\Facades\Http;
use Tests\TestCase;

class MasquePrometheusMetricsTest extends TestCase
{
    use RefreshDatabase;

    public function test_snapshot_parses_vector_response(): void
    {
        Config::set('services.prometheus.url', 'http://prom.test');

        Http::fake([
            'http://prom.test/*' => Http::response([
                'status' => 'success',
                'data' => [
                    'resultType' => 'vector',
                    'result' => [
                        ['metric' => [], 'value' => [1_714_147_381.123, '42']],
                    ],
                ],
            ], 200),
        ]);

        $svc = new MasquePrometheusMetrics;
        $out = $svc->snapshot();

        $this->assertTrue($out['reachable']);
        $this->assertNull($out['error']);
        $this->assertSame(42.0, $out['metrics']['connect_requests']);
    }

    public function test_snapshot_unreachable_when_url_empty(): void
    {
        Config::set('services.prometheus.url', '');

        $out = (new MasquePrometheusMetrics)->snapshot();

        $this->assertFalse($out['reachable']);
        $this->assertNotNull($out['error']);
    }
}
