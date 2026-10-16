function quotaText(className, text) {
  const span = document.createElement('span');
  span.className = className;
  span.textContent = text;
  return span;
}

function renderQuotaMetric(label, metric) {
  const block = document.createElement('div');
  const line = document.createElement('div');
  line.className = 'quota-line';
  line.append(quotaText('', label));
  block.append(line);
  if (!metric) {
    line.append(quotaText('quota-meta', '未提供'));
    return block;
  }
  const remaining = Math.max(0, Math.min(100, metric.remain_percent));
  const tone = remaining === 0 ? ' fail' : remaining <= 20 ? ' warn' : '';
  line.append(quotaText('quota-remaining' + tone, `剩余 ${remaining}%`));
  block.append(quotaText('quota-meta', `重置：${metric.reset_at ? fmt.ts(metric.reset_at) : '未知'}`));
  return block;
}

function renderAccountQuota(snapshot, refreshError = '') {
  const content = document.createElement('div');
  content.className = 'quota-content';
  if (!snapshot.ok) {
    content.append(quotaText('quota-error', snapshot.error || '查询失败，请重试'));
    return content;
  }
  content.append(renderQuotaMetric('5 小时', snapshot.five_hour), renderQuotaMetric('每周', snapshot.weekly));
  if (refreshError) {
    const warning = quotaText('quota-stale', `缓存（刷新失败）：${refreshError}`);
    warning.title = `上次成功：${fmt.ts(snapshot.fetched_at)}；${refreshError}`;
    content.append(warning);
  }
  return content;
}

async function loadCookieQuota(id, force = false, signal = cookieQuotaPoll?.controller.signal) {
  const cell = document.querySelector(`[data-quota-id="${id}"]`);
  if (!cell || signal?.aborted) return;
  if (cell.quotaRequest && !cell.quotaRequest.signal?.aborted) return;
  const request = { signal };
  cell.quotaRequest = request;
  cell.setAttribute('aria-busy', 'true');
  const button = cell.querySelector('button');
  if (button) button.disabled = true;
  if (!cell.quotaSnapshot) cell.replaceChildren(quotaText('quota-meta', '查询中…'));
  try {
    const timeout = AbortSignal.timeout(30000);
    const snapshot = await api(`api/quota?account_id=${id}${force ? '&refresh=1' : ''}`, {
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
    if (!document.contains(cell) || signal?.aborted) return;
    if (snapshot.account_id !== id) throw new Error('额度响应账号校验失败，请重试');
    if (!snapshot.ok) throw new Error(snapshot.error || '查询失败，请重试');
    cell.quotaSnapshot = snapshot;
    cell.replaceChildren(renderAccountQuota(snapshot));
  } catch (error) {
    if (!document.contains(cell) || signal?.aborted) return;
    const message = error.name === 'TimeoutError' ? '查询超时，请稍后重试' : error.message;
    cell.replaceChildren(cell.quotaSnapshot
      ? renderAccountQuota(cell.quotaSnapshot, message)
      : renderAccountQuota({ ok: false, error: message }));
  } finally {
    if (document.contains(cell) && cell.quotaRequest === request) {
      delete cell.quotaRequest;
      cell.removeAttribute('aria-busy');
      cell.querySelector('button')?.remove();
      const refresh = document.createElement('button');
      refresh.className = 'btn muted';
      refresh.textContent = '刷新额度';
      refresh.setAttribute('aria-label', `刷新账号 ${id} 的用量额度`);
      refresh.onclick = () => loadCookieQuota(id, true);
      cell.append(refresh);
    }
  }
}

// 一轮最多三路并发；页面隐藏或离开时结束本轮。
async function loadCookieQuotas(signal) {
  const cells = [...document.querySelectorAll('[data-quota-id]')];
  const worker = async () => {
    while (cells.length && !signal.aborted) {
      const cell = cells.shift();
      if (!document.contains(cell)) return;
      await loadCookieQuota(Number(cell.dataset.quotaId), false, signal);
    }
  };
  await Promise.all(Array.from({ length: Math.min(3, cells.length) }, worker));
}

let cookieQuotaPoll = null;

function stopCookieQuotaPolling() {
  if (!cookieQuotaPoll) return;
  clearTimeout(cookieQuotaPoll.timer);
  cookieQuotaPoll.controller.abort();
  cookieQuotaPoll = null;
}

function startCookieQuotaPolling() {
  stopCookieQuotaPolling();
  if (document.hidden) return;
  const panel = document.querySelector('[data-quota-poll-ms]');
  const state = { controller: new AbortController(), timer: null };
  cookieQuotaPoll = state;
  const poll = async () => {
    await loadCookieQuotas(state.controller.signal);
    // 每轮完成后开始计时，保持各轮串行。
    if (cookieQuotaPoll === state && !document.hidden && document.contains(panel)) {
      state.timer = setTimeout(poll, Number(panel.dataset.quotaPollMs));
    }
  };
  void poll();
}

document.addEventListener('visibilitychange', () => {
  if (document.hidden) stopCookieQuotaPolling();
  else if (currentPage === 'cookies' && document.querySelector('[data-quota-poll-ms]')) startCookieQuotaPolling();
});
