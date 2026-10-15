let rtCfgCache = null, rtModels = [];

// 纯渲染函数保留全部配置字段，网络读取和表单呈现分别维护。
function renderRuntimeConfig(config, models) {
  const renderField = f => {
    const value = config[f.k];
    const id = 'rt_' + f.k;
    const hint = f.hint ? `<div class="rthint" id="${id}_hint">${escapeHtml(f.hint)}</div>` : '';
    const described = f.hint ? ` aria-describedby="${id}_hint"` : '';
    if (f.type === 'bool') {
      return `<div class="cfg-item check">
        <label for="${id}">${escapeHtml(f.label)}</label>
        <input type="checkbox" id="${id}"${described} ${value ? 'checked' : ''}>
        ${hint}</div>`;
    }
    let input;
    if (f.type === 'select') {
      input = `<select id="${id}"${described}>` + models.map(model =>
        `<option value="${escapeHtml(model)}" ${model === value ? 'selected' : ''}>${escapeHtml(model)}</option>`
      ).join('') + '</select>';
    } else if (f.type === 'int') {
      input = `<input type="number" id="${id}"${described} value="${escapeHtml(String(value ?? ''))}" min="${f.min}" max="${f.max}" required>`;
    } else {
      input = `<input type="text" id="${id}"${described} value="${escapeHtml(String(value ?? ''))}">`;
    }
    return `<div class="cfg-item${f.wide ? ' wide' : ''}">
      <label for="${id}">${escapeHtml(f.label)}</label>${input}${hint}</div>`;
  };
  return RT_GROUPS.map(group => {
    const fields = group.fields.filter(field => field.type !== 'bool');
    const toggles = group.fields.filter(field => field.type === 'bool');
    return `<section class="cfg-group">
      <h3>${escapeHtml(group.title)}</h3>
      ${fields.length ? `<div class="cfg-grid">${fields.map(renderField).join('')}</div>` : ''}
      ${toggles.length ? `<div class="cfg-toggle-grid">${toggles.map(renderField).join('')}</div>` : ''}
    </section>`;
  }).join('');
}

async function loadRtCfg() {
  const data = await api('api/config');
  if (!data) return;
  rtCfgCache = data.config; rtModels = data.models || [];
  document.getElementById('rtCfgForm').innerHTML = renderRuntimeConfig(rtCfgCache, rtModels);

  // 三列（项/值/怎么改）挤在窄栏里读不动，第三列换成 title 悬浮
  const d = data.deploy || {};
  document.querySelector('#deployTable tbody').innerHTML = [
    ['监听', `${escapeHtml(d.host || '')}:${d.port}`, 'compose 的 ports + --port'],
    ['SQLite 文件', escapeHtml(d.db_path || ''), '仅 SQLite 模式使用；SQL_DSN 可选择 MySQL 或 PostgreSQL，此处不展示连接凭据'],
    ['Cookie 播种文件', d.cookie_file ? escapeHtml(d.cookie_file) : '<span class="badge muted">未配置</span>',
     '启动时把文件内容导入 Cookie 池；之后从「Cookie 池」页面管理'],
    ['面板鉴权', d.admin_auth ? '<span class="badge ok">已开启</span>'
                             : '<span class="badge fail">未设 token</span>', 'compose 的 ADMIN_TOKEN'],
    ['API Key', d.api_key_locked ? '<span class="badge">部署锁定</span>'
                                 : '<span class="badge ok">面板可轮换</span>', 'compose 的 API_KEY'],
  ].map(r => `<tr><td title="${escapeHtml(r[2])}">${r[0]}</td>
      <td class="pre" style="word-break:break-all;">${r[1]}</td></tr>`).join('');
}

async function saveRtCfg() {
  const msg = document.getElementById('rtCfgMsg');
  const save = document.getElementById('rtCfgSave');
  const reset = document.getElementById('rtCfgReset');
  if (save.disabled) return;
  const body = {};
  for (const f of RT_GROUPS.flatMap(g => g.fields)) {
    const el = document.getElementById('rt_' + f.k);
    if (!el) continue;
    if (!el.checkValidity()) {
      el.reportValidity();
      msg.textContent = '请检查「' + f.label + '」的输入。';
      msg.style.color = 'var(--fail)';
      return;
    }
    body[f.k] = f.type === 'bool' ? el.checked
      : f.type === 'int' ? parseInt(el.value, 10)
      : el.value.trim();
  }
  msg.textContent = '保存中…'; msg.style.color = 'var(--muted)';
  save.disabled = reset.disabled = true;
  try {
    const res = await fetch('api/config', {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || ('HTTP ' + res.status));
    rtCfgCache = data.config;
    msg.textContent = '已保存并生效';
    msg.style.color = 'var(--ok)';
  } catch (error) {
    msg.textContent = '保存失败：' + error.message;
    msg.style.color = 'var(--fail)';
  } finally {
    save.disabled = reset.disabled = false;
  }
}
