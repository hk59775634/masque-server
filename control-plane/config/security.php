<?php

return [
    'web_login' => [
        'max_attempts' => (int) env('WEB_LOGIN_MAX_ATTEMPTS', 5),
        'decay_minutes' => (int) env('WEB_LOGIN_DECAY_MINUTES', 15),
    ],
];
