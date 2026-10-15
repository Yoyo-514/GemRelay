function renderQuotaMetric(label, metric) {
  if (!metric) return `<div class="quota-line"><span>${label}</span><span class="quota-meta">未提供</span></div>`;
  const remaining = Math.max(0, Math.min(100, metric.remain_percent));
  const tone = remaining === 0 ? 'fail' : remaining <= 20 ? 'warn' : '';
  const reset = metric.reset_at ? fmt.ts(metric.reset_at) : '未知';
  return `<div><div class="quota-line"><span>${label}</span>
    <span class="quota-remaining ${tone}">剩余 ${remaining}%</span></div>
    <div class="quota-meta">重置：${escapeHtml(reset)}</div></div>`;
}

function renderAccountQuota(snapshot) {
  if (!snapshot.ok) return `<span class="quota-error">${escapeHtml(snapshot.error || '查询失败，请重试')}</span>`;
  return renderQuotaMetric('5 小时', snapshot.five_hour) + renderQuotaMetric('每周', snapshot.weekly) +
    `<span class="quota-meta">${snapshot.cached ? '缓存' : '更新'}于 ${escapeHtml(fmt.ts(snapshot.fetched_at))}</span>`;
}

async function loadCookieQuota(id, force = false) {
  const cell = document.querySelector(`[data-quota-id="${id}"]`);
  if (!cell || cell.getAttribute('aria-busy') === 'true') return;
  cell.setAttribute('aria-busy', 'true');
  cell.innerHTML = '<span class="quota-meta">查询中…</span><button class="btn muted" disabled>查询中…</button>';
  try {
    const snapshot = await api(`api/quota?account_id=${id}${force ? '&refresh=1' : ''}`, {
      signal: AbortSignal.timeout(30000),
    });
    if (!document.contains(cell)) return;
    if (snapshot.account_id !== id) throw new Error('额度响应与账号不匹配，请重试');
    cell.innerHTML = renderAccountQuota(snapshot);
  } catch (error) {
    if (!document.contains(cell)) return;
    const message = error.name === 'TimeoutError' ? '查询超时，请稍后重试' : error.message;
    cell.innerHTML = `<span class="quota-error">${escapeHtml(message)}</span>`;
  } finally {
    if (document.contains(cell)) {
      cell.removeAttribute('aria-busy');
      cell.insertAdjacentHTML('beforeend', `<button class="btn muted" onclick="loadCookieQuota(${id}, true)" aria-label="刷新账号 ${id} 的用量额度">刷新额度</button>`);
    }
  }
}

// 只在打开 Cookie 池时查询，不定时轮询；最多三路并发，切页后停止未开始的查询。
async function loadCookieQuotas() {
  const cells = [...document.querySelectorAll('[data-quota-id]')];
  const worker = async () => {
    while (cells.length) {
      const cell = cells.shift();
      if (!document.contains(cell)) return;
      await loadCookieQuota(Number(cell.dataset.quotaId));
    }
  };
  await Promise.all(Array.from({ length: Math.min(3, cells.length) }, worker));
}
