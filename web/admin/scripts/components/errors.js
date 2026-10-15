const ERR_BADGE = {
  upstream_rejected: 'muted', rate_limited_local: 'muted', bad_request: 'muted', model_downgraded: 'warn',
  blocked_sorry: 'fail', upstream_429: 'fail', upstream_http_error: 'fail', network: 'fail',
};
const ERR_DESC = {
  upstream_rejected: '上游返回 200 但没有内容帧，属于瞬时拒绝，通常几分钟内自愈；重试即可。',
  rate_limited_local: '请求被本地限流挡下（slot 已满）。在「代理池」添加代理可扩大容量。',
  blocked_sorry: '出口被重定向到 sorry/index。更换出口或等待恢复；历史测试约 106-121 分钟恢复，实际时间随出口变化。',
  model_downgraded: '严格模型校验发现上游实际模型与请求不符。检查账号额度与模型权限。',
  upstream_429: '上游返回 429，通常与出口 IP 有关，可更换代理。',
  upstream_http_error: '上游返回非 200 状态码，可能来自协议变更或临时故障。',
  network: '代理、DNS 或 TLS 握手失败。检查代理可用性；出口质量不稳定的代理会连续失败。',
  bad_request: '客户端请求有问题，例如模型名不存在或传入不支持的字段。',
};


function renderErrorSummary(message) {
  if (!message) return '<span class="muted">—</span>';
  const text = escapeHtml(message);
  return `<div class="error-summary">
    <div class="pre clamp2" title="${text}">${text}</div>
    <button type="button" data-error="${text}" onclick="showErrorDetail(this.dataset.error)">查看完整错误</button>
  </div>`;
}

function showErrorDetail(message) {
  const text = String(message ?? '');
  return buildModal({
    title: '错误详情',
    desc: '展示后端返回的全部错误文本。旧记录中已截断的内容无法恢复；上游原始响应可能仅保留安全片段。',
    bodyHTML: `<pre class="error-detail">${escapeHtml(text)}</pre><div class="hint" data-copy-status role="status"></div>`,
    submitText: '复制全文', cancelText: '关闭', wide: true, dismissOnBackdrop: false,
    onSubmit: async overlay => {
      try {
        await copyText(text);
        overlay.querySelector('[data-copy-status]').textContent = '已复制完整文本';
      } catch (error) {
        return { error: '复制失败，请手动选择并复制文本。' };
      }
      return false;
    },
  });
}
