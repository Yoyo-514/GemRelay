// 本地文件即可运行；不启动服务器、不替换生产函数，也不访问 Google。
window.addEventListener('error', event => {
  document.documentElement.dataset.testStatus = 'failed';
  document.documentElement.dataset.testError = event.message;
});

function assertUI(condition, message) {
  if (!condition) throw new Error(message);
}

try {
  const failed = renderAccountQuota({ ok: false, error: '<img src=x onerror=alert(1)>请重试' });
  assertUI(failed.includes('&lt;img') && !failed.includes('<img'), '额度错误必须转义');
  assertUI(!failed.includes('剩余 0%'), '查询失败不能显示为额度耗尽');
  const partial = renderAccountQuota({ ok: true, fetched_at: 1792080000,
    five_hour: { remain_percent: 37, reset_at: 1792093504 } });
  assertUI(partial.includes('37%') && partial.includes('未提供'), '缺失档位不能伪造');
  assertUI(renderQuotaMetric('5 小时', { remain_percent: 0, reset_at: 1792093504 }).includes('fail'), '耗尽需要文本与状态提示');
  assertUI(renderQuotaMetric('每周', { remain_percent: 13, reset_at: 1792093504 }).includes('warn'), '低额度应突出显示');
  const fields = RT_GROUPS.flatMap(group => group.fields);
  assertUI(new Set(fields.map(field => field.k)).size === fields.length, '设置字段不能重复');
  const form = renderRuntimeConfig({ strict_model: true }, ['gemini-3.6-flash']);
  document.getElementById('app').innerHTML = form;
  assertUI(document.getElementById('rt_strict_model')?.checked, '严格模型开关应显示保存值');
  assertUI(parseCookieStr('SAPISID=a=b; SID=x').SAPISID === 'a=b', '拆分后 Cookie 解析应保留等号');

  document.getElementById('app').innerHTML = renderShellMarkup(false);
  document.getElementById('pageTitle').textContent = 'Cookie 池';
  const items = [
    { id: 1, label: '主账号', status: 'enabled', health: 'ok', has_1psidts: true },
    { id: 2, label: '备用账号', status: 'disabled', health: 'disabled', has_1psidts: true },
    { id: 3, label: '等待重新登录', status: 'enabled', health: 'dead', fail_count: 1 },
    { id: 4, label: '<script>不可执行</script>', status: 'enabled', health: 'unknown' },
  ].map(item => ({ cookie_count: 9, key_cookies: ['SAPISID', 'SID'], ...item }));
  document.getElementById('main').innerHTML = renderCookiePool({ items, total: 4, enabled: 3 });
  labelTableCells(document.getElementById('main'));
  document.querySelectorAll('nav button').forEach(button => {
    button.classList.toggle('active', button.dataset.page === 'cookies');
  });
  const states = [
    { ok: true, fetched_at: 1792080000, five_hour: { remain_percent: 37, reset_at: 1792093504 }, weekly: { remain_percent: 87, reset_at: 1792583104 } },
    { ok: true, cached: true, fetched_at: 1792080000, five_hour: { remain_percent: 0, reset_at: 1792093504 }, weekly: { remain_percent: 13, reset_at: 1792583104 } },
    { ok: false, error: '取 XSRF token 失败，请检查账号登录态与出口连接' },
    { ok: true, fetched_at: 1792080000, five_hour: { remain_percent: 100, reset_at: 1792093504 } },
  ];
  document.querySelectorAll('[data-quota-id]').forEach((cell, index) => {
    cell.innerHTML = renderAccountQuota(states[index]);
    const button = document.createElement('button');
    button.className = 'btn muted';
    button.textContent = '刷新额度';
    cell.appendChild(button);
  });
  assertUI(document.querySelectorAll('.quota-cell').length === 4, '每个账号应有额度区域');
  assertUI(!document.querySelector('#main script'), '账号标签不能注入脚本');
  const main = document.getElementById('main');
  assertUI(main.scrollWidth <= main.clientWidth + 1, '表格只能在自身容器内横向滚动');
  if (window.innerWidth <= 600) {
    const quota = document.querySelector('.quota-cell').getBoundingClientRect();
    assertUI(quota.left >= 0 && quota.right <= window.innerWidth, '手机额度必须直接可见');
    assertUI(getComputedStyle(document.querySelector('nav')).position === 'fixed', '手机导航应固定在底部');
  } else {
    assertUI(getComputedStyle(document.querySelector('.mobile-logout')).display === 'none', '桌面不应重复显示手机退出入口');
  }
  const surface = new URLSearchParams(location.search).get('surface') || 'cookies';
  if (surface === 'settings') {
    main.innerHTML = renderSettingsPage({ key: 'sk-example-only', locked: false });
    document.getElementById('rtCfgForm').innerHTML = renderRuntimeConfig({
      strict_model: true, default_model: 'gemini-3.6-flash', retry_attempts: 3,
      request_timeout_sec: 180, max_prompt_bytes: 128000, impersonate: 'chrome_146',
    }, ['gemini-3.6-flash']);
    document.getElementById('pageTitle').textContent = '设置';
  } else if (surface === 'proxies') {
    main.innerHTML = renderProxyPool({ items: [
      { id: 1, name: '住宅出口', url: 'http://proxy.example:8080', enabled: true, fail_count: 0, last_error: '' },
      { id: 2, name: '备用出口', url: 'socks5://backup.example:1080', enabled: false, fail_count: 5, last_error: '连接超时，请检查出口后重试' },
    ] });
    labelTableCells(main);
    document.getElementById('pageTitle').textContent = '代理池';
  }
  document.querySelectorAll('nav button').forEach(button => button.classList.toggle('active', button.dataset.page === surface));
  assertUI(main.scrollWidth <= main.clientWidth + 1, '业务页面不能横向溢出');
  document.documentElement.dataset.testStatus = 'passed';
  document.documentElement.dataset.testViewport = `${window.innerWidth}x${window.innerHeight}`;
} catch (error) {
  document.documentElement.dataset.testStatus = 'failed';
  document.documentElement.dataset.testError = error.message;
  throw error;
}
