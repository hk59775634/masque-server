<?php

return [

    /*
    |--------------------------------------------------------------------------
    | Third Party Services
    |--------------------------------------------------------------------------
    |
    | This file is for storing the credentials for third party services such
    | as Mailgun, Postmark, AWS and more. This file provides the de facto
    | location for this type of information, allowing packages to have
    | a conventional file to locate the various service credentials.
    |
    */

    'postmark' => [
        'key' => env('POSTMARK_API_KEY'),
    ],

    'resend' => [
        'key' => env('RESEND_API_KEY'),
    ],

    'ses' => [
        'key' => env('AWS_ACCESS_KEY_ID'),
        'secret' => env('AWS_SECRET_ACCESS_KEY'),
        'region' => env('AWS_DEFAULT_REGION', 'us-east-1'),
    ],

    'slack' => [
        'notifications' => [
            'bot_user_oauth_token' => env('SLACK_BOT_USER_OAUTH_TOKEN'),
            'channel' => env('SLACK_BOT_USER_DEFAULT_CHANNEL'),
        ],
    ],

    /*
    | Prometheus HTTP API (Admin 运营概览). Leave empty to skip remote queries.
    */
    /*
    | Device provisioning: base URL of masque-server (POST /connect). Not the Laravel APP_URL.
    */
    'masque' => [
        'server_url' => rtrim((string) env('MASQUE_SERVER_URL', 'http://127.0.0.1:8443'), '/'),
        // Optional HMAC signature verification for POST /api/v1/server/authorize
        // Header contract:
        // - X-Masque-Authz-Timestamp: unix seconds
        // - X-Masque-Authz-Signature: HMAC-SHA256(method+"\n"+path+"\n"+ts+"\n"+sha256(body))
        'authorize_hmac_secret' => (string) env('MASQUE_AUTHORIZE_HMAC_SECRET', ''),
        'authorize_hmac_required' => filter_var(env('MASQUE_AUTHORIZE_HMAC_REQUIRED', false), FILTER_VALIDATE_BOOL),
        'authorize_hmac_window_seconds' => (int) env('MASQUE_AUTHORIZE_HMAC_WINDOW_SECONDS', 300),
    ],

    'prometheus' => [
        'url' => env('PROMETHEUS_URL', 'http://127.0.0.1:9090'),
    ],

    /*
    | 观测栈 Web UI（Admin 运营概览快捷链接，与 ops/observability/docker-compose 默认端口一致）
    */
    'observability' => [
        'grafana_url' => env('GRAFANA_URL', 'http://127.0.0.1:3000'),
        'prometheus_ui_url' => env('PROMETHEUS_UI_URL', env('PROMETHEUS_URL', 'http://127.0.0.1:9090')),
        'alertmanager_url' => env('ALERTMANAGER_URL', 'http://127.0.0.1:9093'),
        'loki_url' => env('LOKI_URL', 'http://127.0.0.1:3100'),
    ],

];
