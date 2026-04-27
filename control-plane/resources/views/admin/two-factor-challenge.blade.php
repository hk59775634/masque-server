<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>管理员两步验证 - MASQUE</title>
    <style>
        body { margin: 0; font-family: "Segoe UI", "PingFang SC", sans-serif; background: #0b1020; color: #dbe6ff; }
        .box { max-width: 420px; margin: 64px auto; padding: 28px; border-radius: 14px; border: 1px solid #243557; background: #111a2f; }
        h1 { margin: 0 0 8px; font-size: 22px; }
        .muted { color: #8ea2c6; font-size: 14px; margin-bottom: 18px; }
        label { display: block; margin-bottom: 8px; font-size: 14px; }
        input { width: 100%; padding: 12px; border-radius: 10px; border: 1px solid #30486f; background: #060b18; color: #dbe6ff; font-size: 18px; letter-spacing: 0.2em; text-align: center; }
        button { margin-top: 16px; width: 100%; padding: 12px; border: none; border-radius: 10px; background: linear-gradient(120deg, #7dd3fc, #60a5fa); color: #041428; font-weight: 700; cursor: pointer; }
        .err { background: #3f1118; border: 1px solid #7f1d1d; color: #fecaca; padding: 10px 12px; border-radius: 10px; margin-bottom: 12px; font-size: 14px; }
        .ok { background: #0f2f34; border: 1px solid #22796e; color: #91f7eb; padding: 10px 12px; border-radius: 10px; margin-bottom: 12px; font-size: 14px; }
    </style>
</head>
<body>
<div class="box">
    <h1>两步验证</h1>
    <p class="muted">请输入身份验证器应用中的 6 位数字验证码。</p>
    @if(session('status'))
        <div class="ok">{{ session('status') }}</div>
    @endif
    @if($errors->any())
        <div class="err">{{ $errors->first() }}</div>
    @endif
    <form method="POST" action="{{ route('admin.two-factor.verify') }}">
        @csrf
        <label for="code">验证码</label>
        <input id="code" name="code" type="text" inputmode="numeric" pattern="[0-9]*" maxlength="6" autocomplete="one-time-code" autofocus required>
        <button type="submit">继续</button>
    </form>
</div>
</body>
</html>
