async function editCookieAcct(id, label, note) {
  const data = await modalForm({
    title: '编辑 Cookie #' + id,
    desc: '只改标签和备注，不动 cookie 本身。要换 cookie 请删掉重新导入。',
    fields: [
      { name: 'label', label: '标签', value: label || '', placeholder: '账号邮箱' },
      { name: 'note', label: '备注', value: note || '', placeholder: '可留空' },
    ],
  });
  if (!data) return;
  const j = await api('api/cookies/' + id, {
    method: 'PATCH', body: JSON.stringify({ label: data.label, note: data.note }),
  });
  if (j.error) return flash('保存失败：' + j.error);
  flash('已保存');
  await refreshPage();
}

// 检测 cookie 还有没有登录态。判据是 /app 页面里有没有 SNlM0e——cookie 失效时
// Gemini 不报错、只是把你当匿名，纯文本请求照样 200，所以不能拿"请求成功"当判据。
async function checkCookieAcct(id, btn) {
  const old = btn.textContent;
  btn.textContent = '检测中…'; btn.disabled = true;
  try {
    const r = await fetch('api/cookies/' + id + '/check',
      { method: 'POST', credentials: 'include' });
    const d = await r.json().catch(() => ({}));
    if (!r.ok) { flash('检测失败：' + (d.error || r.status)); return; }
    const via = d.proxy_name ? `，出口 ${d.proxy_name}` : '';
    flash((d.ok ? '✓ #' + id + ' ' : '✗ #' + id + ' ') + (d.detail || '') +
          `${via}，${d.took_ms}ms`);
    await refreshPage();
  } finally {
    btn.textContent = old; btn.disabled = false;
  }
}

// 手动触发一次会话保活（刷新 1PSIDTS + SIDCC）。后台启动 15 秒后跑第一次，之后每 10 分钟。
async function rotateCookieAcct(id, btn) {
  const old = btn.textContent;
  btn.textContent = '…'; btn.disabled = true;
  try {
    const r = await fetch('api/cookies/' + id + '/rotate',
      { method: 'POST', credentials: 'include' });
    const d = await r.json().catch(() => ({}));
    if (!r.ok) { flash('保活失败：' + (d.error || r.status)); return; }
    flash((d.ok ? '✓ #' + id + ' ' : '✗ #' + id + ' ') + (d.detail || '') +
          (d.next_sec ? `，服务端建议 ${d.next_sec}s 后再来` : ''));
    await refreshPage();
  } finally { btn.textContent = old; btn.disabled = false; }
}

async function toggleCookieAcct(id) {
  await api('api/cookies/' + id + '/toggle', { method: 'POST' });
  await refreshPage();
}

async function delCookieAcct(id) {
  const ok = await modalConfirm({
    title: '删除 Cookie',
    desc: `确定删除账号 #${id}？此操作不可撤销，删除后请求不再用它。`,
    confirmText: '删除', danger: true,
  });
  if (!ok) return;
  await api('api/cookies/' + id, { method: 'DELETE' });
  flash('已删除');
  await refreshPage();
}
