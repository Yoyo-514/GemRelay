const ERR_BADGE = {
  upstream_rejected: 'muted', rate_limited_local: 'muted', bad_request: 'muted', model_downgraded: 'warn',
  blocked_sorry: 'fail', upstream_429: 'fail', upstream_http_error: 'fail', network: 'fail',
};
const ERR_DESC = {
  upstream_rejected: '上游返回 200 但没有内容帧。实测是瞬时拒绝，隔几分钟自愈，跟频率/并发/累积次数都无关 —— 重试即可，不用调限流。',
  rate_limited_local: '本地限流挡下的（slot 满）。要更高产能就在「代理池」加代理。',
  blocked_sorry: '被 Google 重定向到 sorry/index，当前出口受限。可更换出口或等待恢复；历史测试约 106-121 分钟恢复，不保证每次相同。',
  model_downgraded: '严格模型校验发现上游实际模型与请求不符。检查账号额度和模型权限。',
  upstream_429: '上游直接 429。同样是出口 IP 嫌疑，换代理。',
  upstream_http_error: '上游返回非 200 的其它状态码，可能是协议变更或临时故障。',
  network: '连不上代理 / DNS / TLS 握手失败。检查代理可用性 —— 实测代理服务商出口质量参差，劣质出口会连续失败。',
  bad_request: '客户端请求本身有问题（模型名不存在、传了不支持的字段）。',
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
