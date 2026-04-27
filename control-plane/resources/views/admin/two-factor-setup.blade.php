<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>两步验证设置 - MASQUE</title>
    <style>
        body { margin: 0; font-family: "Segoe UI", "PingFang SC", sans-serif; background: #0b1020; color: #dbe6ff; }
        .wrap { max-width: 640px; margin: 40px auto; padding: 0 16px; }
        .card { background: #111a2f; border: 1px solid #243557; border-radius: 14px; padding: 22px; margin-bottom: 16px; }
        h1 { margin: 0 0 8px; font-size: 24px; }
        .muted { color: #8ea2c6; font-size: 14px; margin-bottom: 14px; }
        pre { background: #050a16; border: 1px solid #2a3e65; padding: 12px; border-radius: 10px; overflow: auto; word-break: break-all; font-size: 13px; }
        label { display: block; margin: 12px 0 6px; font-size: 14px; }
        input { width: 100%; padding: 10px; border-radius: 10px; border: 1px solid #30486f; background: #060b18; color: #dbe6ff; }
        button, .btn { display: inline-block; margin-top: 12px; padding: 10px 16px; border-radius: 10px; border: none; font-weight: 700; cursor: pointer; text-decoration: none; }
        .primary { background: linear-gradient(120deg, #7dd3fc, #60a5fa); color: #041428; }
        .danger { background: #4b2432; color: #fecdd3; border: 1px solid #7f1d1d; }
        .secondary { background: #13284f; color: #c7dcff; border: 1px solid #30548a; }
        .ok { background: #0f2f34; border: 1px solid #22796e; color: #91f7eb; padding: 10px 12px; border-radius: 10px; margin-bottom: 12px; }
        .err { background: #3f1118; border: 1px solid #7f1d1d; color: #fecaca; padding: 10px 12px; border-radius: 10px; margin-bottom: 12px; }
    </style>
</head>
<body>
<div class="wrap">
    <div class="card">
        <h1>管理员两步验证（TOTP）</h1>
        <p class="muted">使用 Google Authenticator、1Password 等应用扫描或手动输入密钥。关闭前请先生成高危操作一次性确认码。</p>
        <a class="btn secondary" href="{{ route('admin.index', ['tab' => 'overview']) }}">返回管理后台</a>
    </div>

    @if(session('status'))
        <div class="ok">{{ session('status') }}</div>
    @endif
    @if($errors->any())
        <div class="err">{{ $errors->first() }}</div>
    @endif

    @if($enabled)
        <div class="card">
            <h2 style="margin-top:0;">两步验证已启用</h2>
            <p class="muted">关闭后管理入口将不再要求验证码（需一次性确认码）。</p>
            <form method="POST" action="{{ route('admin.two-factor.disable') }}" onsubmit="return confirm('确认关闭两步验证？');">
                @csrf
                <label for="operation_token">一次性确认码</label>
                <input id="operation_token" name="operation_token" type="text" required placeholder="在管理后台生成">
                <button class="danger" type="submit">关闭两步验证</button>
            </form>
        </div>
    @else
        <div class="card">
            <h2 style="margin-top:0;">1. 在验证器中添加账号</h2>
            <p class="muted">密钥（手动输入）</p>
            <pre>{{ $plainSecret }}</pre>
            <p class="muted">otpauth URI（部分应用支持从剪贴板导入）</p>
            <pre>{{ $otpauthUrl }}</pre>
        </div>
        <div class="card">
            <h2 style="margin-top:0;">2. 输入 6 位验证码以确认启用</h2>
            <form method="POST" action="{{ route('admin.two-factor.setup.confirm') }}">
                @csrf
                <label for="code">验证码</label>
                <input id="code" name="code" type="text" inputmode="numeric" pattern="[0-9]*" maxlength="6" autocomplete="one-time-code" required>
                <button class="primary" type="submit">启用两步验证</button>
            </form>
        </div>
    @endif
</div>
</body>
</html>
