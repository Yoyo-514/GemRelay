// API key actions ──────────────────────────────────────────────────────────

function toggleKeyVis() {
  const el = document.getElementById('apiKeyView');
  const btn = el.parentElement.querySelector('button');
  el.type = el.type === 'password' ? 'text' : 'password';
  if (btn) btn.textContent = el.type === 'password' ? '显示' : '隐藏';
}

async function copyKey() {
  const el = document.getElementById('apiKeyView');
  try {
    await copyText(el.value);
    flash('已复制到剪贴板');
  } catch (e) {
    flash('复制失败，请手动选择并复制密钥。');
  }
}

async function rotateKey() {
  const ok = await modalConfirm({
    title: '生成新 Key',
    desc: '现有客户端在重新配置前将无法访问。确定继续？',
    confirmText: '生成新 Key',
    danger: true,
  });
  if (!ok) return;
  await api('api/apikey', { method: 'POST' });
  flash('已生成新 Key');
  await refreshPage();
  const el = document.getElementById('apiKeyView');
  if (el) { el.type = 'text'; }
}

async function customKey() {
  const k = await modalPrompt({
    title: '设置自定义 API Key',
    desc: '至少 20 字符。设置后所有客户端要更换为新 Key。',
    label: '新 Key',
    placeholder: 'sk-gemrelay-...',
    password: true,
  });
  if (!k) return;
  if (k.length < 20) return flash('Key 至少 20 字符');
  const r = await fetch('api/apikey', {
    method: 'PATCH', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key: k }),
  });
  const j = await r.json();
  if (j.ok) { flash('Key 已更新'); await refreshPage(); }
  else flash('错误：' + (j.error || '未知错误'));
}
