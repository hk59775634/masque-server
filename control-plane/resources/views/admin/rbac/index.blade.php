<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>RBAC 管理 - MASQUE</title>
    <style>
        :root { --bg:#0b1020;--line:#243557;--text:#dbe6ff;--muted:#8ea2c6;--accent:#63b3ff;}
        *{box-sizing:border-box}
        body{margin:0;background:var(--bg);font-family:"Segoe UI","PingFang SC",sans-serif;color:var(--text);padding:24px 16px}
        .wrap{max-width:1100px;margin:0 auto}
        .top{display:flex;justify-content:space-between;align-items:flex-start;gap:12px;flex-wrap:wrap;margin-bottom:16px}
        h1{margin:0;font-size:24px}
        .muted{color:var(--muted);font-size:14px;margin-top:6px}
        .btn{display:inline-flex;padding:9px 12px;border-radius:10px;border:1px solid var(--line);color:var(--text);background:#0b1222;text-decoration:none;font-size:14px}
        .card{border:1px solid var(--line);border-radius:14px;padding:14px;margin-bottom:14px;background:#111a2f}
        .card h2{margin:0 0 10px;font-size:17px}
        .ok{margin:8px 0;padding:8px 10px;border:1px solid #22796e;background:#0f2f34;border-radius:10px;color:#91f7eb;font-size:14px}
        .err{margin:8px 0;padding:8px 10px;border:1px solid #7f1d1d;background:#3f1118;border-radius:10px;color:#fecaca;font-size:14px}
        label{display:block;font-size:13px;color:#aac1e8;margin:6px 0 4px}
        input,select,button,textarea{font:inherit;color:var(--text)}
        input,select,textarea{width:100%;max-width:420px;padding:8px 10px;border-radius:9px;border:1px solid #30486f;background:#060b18}
        select[multiple]{max-width:100%;min-height:140px}
        .save{margin-top:8px;padding:9px 14px;border:none;border-radius:10px;background:linear-gradient(120deg,#7dd3fc,#60a5fa);color:#041428;font-weight:700;cursor:pointer}
        .hint{font-size:12px;color:var(--muted);margin-top:6px}
        ul.compact{margin:6px 0 0;padding-left:18px;color:var(--muted);font-size:13px}
    </style>
</head>
<body>
<div class="wrap">
    <div class="top">
        <div>
            <h1>角色与权限</h1>
            <div class="muted">创建自定义角色、为角色绑定权限；内置模板：auditor / ops / security。变更 admin 角色或授予 admin.rbac.write 需一次性确认码。</div>
        </div>
        <div style="display:flex;gap:8px;flex-wrap:wrap">
            <a class="btn" href="{{ route('admin.index') }}">返回控制台</a>
            <a class="btn" href="{{ route('admin.index', ['tab' => 'users']) }}">用户策略（细粒度）</a>
        </div>
    </div>

    @if(session('status'))
        <div class="ok">{{ session('status') }}</div>
    @endif
    @if($errors->any())
        <div class="err">
            @foreach($errors->all() as $e){{ $e }}@if(!$loop->last)<br>@endif @endforeach
        </div>
    @endif

    <div class="card">
        <h2>新建角色</h2>
        <form method="POST" action="{{ route('admin.rbac.roles.store') }}">
            @csrf
            <label>内部名（小写、数字、连字符）</label>
            <input name="name" required pattern="[a-z0-9][a-z0-9_-]*" maxlength="64" placeholder="例如 custom_support">
            <label>显示名（可选）</label>
            <input name="display_name" maxlength="120" placeholder="工单支持">
            <button class="save" type="submit">创建</button>
        </form>
    </div>

    <div class="card">
        <h2>按角色绑定权限</h2>
        <p class="hint">使用 Ctrl/Cmd 多选权限。下列情况需在对应表单填写「一次性确认码」（控制台首页签发）：修改 <strong>admin</strong> 角色绑定；向任意角色<strong>新授予</strong>敏感权限（<code>admin.access</code>、<code>admin.policy.write</code>、<code>admin.session.revoke</code>、<code>admin.rbac.write</code>）。</p>
        @foreach($roles as $role)
            <form method="POST" action="{{ route('admin.rbac.roles.permissions', $role) }}" style="margin-bottom:18px;padding-bottom:14px;border-bottom:1px solid #22365d">
                @csrf
                <strong>{{ $role->name }}</strong>
                @if($role->display_name)<span class="muted"> — {{ $role->display_name }}</span>@endif
                <label>权限（多选）</label>
                <select name="permission_ids[]" multiple required>
                    @foreach($permissions as $p)
                        <option value="{{ $p->id }}" @selected($role->permissions->contains('id', $p->id))>
                            {{ $p->name }} @if($p->display_name) ({{ $p->display_name }}) @endif
                        </option>
                    @endforeach
                </select>
                <label>一次性确认码（高危时必填）</label>
                <input name="operation_token" type="text" maxlength="32" autocomplete="off" placeholder="可选">
                <button class="save" type="submit">保存 {{ $role->name }}</button>
            </form>
        @endforeach
    </div>

    <div class="card">
        <h2>按用户同步角色</h2>
        <p class="hint">仅同步角色与 is_admin 标志；路由/ACL 等请在「用户策略」页修改。涉及管理员特权变化时需一次性确认码。</p>
        <form method="POST" action="{{ route('admin.rbac.users.roles') }}">
            @csrf
            <label>用户</label>
            <select name="user_id" required>
                <option value="">— 选择用户 —</option>
                @foreach($users as $u)
                    <option value="{{ $u->id }}">#{{ $u->id }} {{ $u->email }}</option>
                @endforeach
            </select>
            <label>角色</label>
            @foreach($roles as $role)
                <label style="display:flex;align-items:center;gap:8px;font-weight:normal">
                    <input type="checkbox" name="role_ids[]" value="{{ $role->id }}"> {{ $role->name }}
                </label>
            @endforeach
            <label>is_admin</label>
            <select name="is_admin_mode">
                <option value="keep" selected>保持当前</option>
                <option value="1">设为 true（高危；常配合 admin 角色）</option>
                <option value="0">设为 false</option>
            </select>
            <p class="hint">当设为 true 时，若未勾选 admin 角色，系统会自动附加 admin 角色（与策略页一致）。</p>
            <label>一次性确认码（高危时必填）</label>
            <input name="operation_token" type="text" maxlength="32" autocomplete="off" placeholder="可选">
            <button class="save" type="submit">同步用户角色</button>
        </form>
    </div>

    <div class="card">
        <h2>模板说明</h2>
        <ul class="compact">
            <li><strong>auditor</strong>：admin.access + admin.audit.read</li>
            <li><strong>ops</strong>：+ admin.policy.write</li>
            <li><strong>security</strong>：+ admin.session.revoke</li>
            <li><strong>admin</strong>：全权限（含 admin.rbac.write）</li>
        </ul>
    </div>
</div>
</body>
</html>
