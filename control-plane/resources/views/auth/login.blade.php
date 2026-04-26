<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>登录 - MASQUE 控制台</title>
    <style>
        body { margin: 0; font-family: Arial, sans-serif; background: #f5f7fb; color: #1f2937; }
        .container { max-width: 440px; margin: 48px auto; background: #fff; border-radius: 12px; padding: 28px; box-shadow: 0 12px 30px rgba(0, 0, 0, 0.08); }
        h1 { margin-top: 0; margin-bottom: 20px; font-size: 24px; }
        label { display: block; margin-bottom: 6px; font-weight: 600; font-size: 14px; }
        input[type="email"], input[type="password"] { width: 100%; padding: 10px 12px; border: 1px solid #d1d5db; border-radius: 8px; margin-bottom: 14px; box-sizing: border-box; }
        button { width: 100%; border: none; border-radius: 8px; padding: 11px 14px; background: #2563eb; color: #fff; font-weight: 700; cursor: pointer; }
        .notice { padding: 10px 12px; border-radius: 8px; margin-bottom: 14px; font-size: 14px; }
        .success { background: #ecfdf3; color: #166534; border: 1px solid #86efac; }
        .error { background: #fef2f2; color: #991b1b; border: 1px solid #fca5a5; }
        .row { margin-top: 10px; display: flex; justify-content: space-between; align-items: center; font-size: 14px; }
        .row a { color: #2563eb; text-decoration: none; }
    </style>
</head>
<body>
<div class="container">
    <h1>账号登录</h1>

    @if(session('status'))
        <div class="notice success">{{ session('status') }}</div>
    @endif

    @if($errors->any())
        <div class="notice error">
            <ul style="margin: 0; padding-left: 18px;">
                @foreach($errors->all() as $error)
                    <li>{{ $error }}</li>
                @endforeach
            </ul>
        </div>
    @endif

    <form method="POST" action="{{ route('login.store') }}">
        @csrf

        <label for="email">邮箱</label>
        <input id="email" type="email" name="email" value="{{ old('email') }}" required>

        <label for="password">密码</label>
        <input id="password" type="password" name="password" required>

        <label style="display:flex;gap:8px;align-items:center;margin-bottom:14px;">
            <input type="checkbox" name="remember" style="margin:0;">
            记住我
        </label>

        <button type="submit">登录</button>
    </form>

    <div class="row">
        <span>还没有账号？</span>
        <a href="{{ route('register') }}">去注册</a>
    </div>
</div>
</body>
</html>
