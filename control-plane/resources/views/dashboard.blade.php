<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>用户首页 - MASQUE 控制台</title>
    <style>
        body { margin: 0; font-family: Arial, sans-serif; background: #f5f7fb; color: #111827; }
        .container { max-width: 900px; margin: 40px auto; padding: 0 16px; }
        .card { background: #fff; border-radius: 12px; box-shadow: 0 8px 24px rgba(0,0,0,0.08); padding: 24px; margin-bottom: 16px; }
        h1 { margin: 0 0 6px; font-size: 28px; }
        .muted { color: #6b7280; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; margin-top: 14px; }
        .box { background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 10px; padding: 14px; }
        .actions { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 16px; }
        a, button { text-decoration: none; border: none; border-radius: 8px; padding: 10px 14px; font-weight: 700; cursor: pointer; }
        .primary { background: #2563eb; color: #fff; }
        .secondary { background: #fff; color: #111827; border: 1px solid #d1d5db; }
        .success { background: #ecfdf3; color: #166534; border: 1px solid #86efac; padding: 10px 12px; border-radius: 8px; margin-bottom: 12px; }
    </style>
</head>
<body>
<div class="container">
    <div class="card">
        @if(session('status'))
            <div class="success">{{ session('status') }}</div>
        @endif

        <h1>欢迎，{{ auth()->user()->name }}</h1>
        <p class="muted">这是你的用户登录首页（Dashboard）。</p>

        <div class="grid">
            <div class="box">
                <strong>登录邮箱</strong>
                <div class="muted" style="margin-top:6px;">{{ auth()->user()->email }}</div>
            </div>
            <div class="box">
                <strong>账号ID</strong>
                <div class="muted" style="margin-top:6px;">{{ auth()->id() }}</div>
            </div>
            <div class="box">
                <strong>下一步</strong>
                <div class="muted" style="margin-top:6px;">后续这里可接入设备管理、配置下载、连接状态。</div>
            </div>
        </div>

        <div class="actions">
            <a class="secondary" href="{{ url('/') }}">返回首页</a>
            @if(auth()->user()?->is_admin)
                <a class="secondary" href="{{ route('admin.index') }}">Admin 管理</a>
            @endif
            <form method="POST" action="{{ route('logout') }}" style="margin:0;">
                @csrf
                <button class="primary" type="submit">退出登录</button>
            </form>
        </div>
    </div>
</div>
</body>
</html>
