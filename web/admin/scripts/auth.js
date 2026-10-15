async function doLogout() {
  const ok = await modalConfirm({
    title: '退出登录',
    desc: '退出后需要重新输入管理员 token 才能进来。服务本身不受影响，API 照常提供。',
    confirmText: '退出',
    danger: true,
  });
  if (!ok) return;
  await api('api/logout', { method: 'POST' });
  currentPage = null;
  renderLogin();
}

function renderLogin() {
  document.getElementById('app').innerHTML = `
    <form class="login-box" onsubmit="doLogin(event)">
      <h1><span class="spark" style="width:20px;height:20px;background:var(--g-sweep);
        border-radius:4px;display:inline-block;
        clip-path:polygon(50% 0,61% 39%,100% 50%,61% 61%,50% 100%,39% 61%,0 50%,39% 39%)"></span>
        gemini-web2api</h1>
      <p class="lede" id="loginHint">登录后管理请求记录、代理出口和账号状态。</p>
      <label for="tokenInput">管理员令牌</label>
      <input id="tokenInput" type="password" placeholder="输入管理员令牌" autocomplete="off" aria-describedby="loginHint loginErr" required />
      <button class="btn primary" id="loginBtn" type="submit">登录管理面板</button>
      <div id="loginErr" class="err" role="alert"></div>
    </form>`;
  document.getElementById('tokenInput').focus();
}

async function doLogin(event) {
  if (event) event.preventDefault();
  const button = document.getElementById('loginBtn');
  if (button.disabled) return;
  const input = document.getElementById('tokenInput');
  if (!input.reportValidity()) return;
  const error = document.getElementById('loginErr');
  error.textContent = '';
  button.disabled = true;
  button.textContent = '正在登录…';
  try {
    const response = await fetch('api/login', { method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: input.value }) });
    if (response.ok) renderShell();
    else error.textContent = response.status === 401 ? '管理员令牌不正确，请重新输入。' : '登录失败：HTTP ' + response.status;
  } catch (failure) {
    error.textContent = '连接失败，请检查服务地址或网络后重试。';
  } finally {
    button.disabled = false;
    button.textContent = '登录管理面板';
  }
}
