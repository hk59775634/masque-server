<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>用户注册 - MASQUE 控制台</title>
    <style>
        body {
            margin: 0;
            font-family: Arial, sans-serif;
            background: #f5f7fb;
            color: #1f2937;
        }
        .container {
            max-width: 440px;
            margin: 48px auto;
            background: #fff;
            border-radius: 12px;
            padding: 28px;
            box-shadow: 0 12px 30px rgba(0, 0, 0, 0.08);
        }
        h1 {
            margin-top: 0;
            margin-bottom: 20px;
            font-size: 24px;
        }
        label {
            display: block;
            margin-bottom: 6px;
            font-weight: 600;
            font-size: 14px;
        }
        input {
            width: 100%;
            padding: 10px 12px;
            border: 1px solid #d1d5db;
            border-radius: 8px;
            margin-bottom: 14px;
            box-sizing: border-box;
        }
        button {
            width: 100%;
            border: none;
            border-radius: 8px;
            padding: 11px 14px;
            background: #2563eb;
            color: #fff;
            font-weight: 700;
            cursor: pointer;
        }
        .notice {
            padding: 10px 12px;
            border-radius: 8px;
            margin-bottom: 14px;
            font-size: 14px;
        }
        .success {
            background: #ecfdf3;
            color: #166534;
            border: 1px solid #86efac;
        }
        .error {
            background: #fef2f2;
            color: #991b1b;
            border: 1px solid #fca5a5;
        }
    </style>
</head>
<body>
<div class="container">
    <h1>注册账号</h1>

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

    <form method="POST" action="{{ route('register.store') }}">
        @csrf

        <label for="name">姓名</label>
        <input id="name" type="text" name="name" value="{{ old('name') }}" required>

        <label for="email">邮箱</label>
        <input id="email" type="email" name="email" value="{{ old('email') }}" required>

        <label for="password">密码</label>
        <input id="password" type="password" name="password" required>

        <label for="password_confirmation">确认密码</label>
        <input id="password_confirmation" type="password" name="password_confirmation" required>

        <button type="submit">立即注册</button>
    </form>
</div>
</body>
</html>
