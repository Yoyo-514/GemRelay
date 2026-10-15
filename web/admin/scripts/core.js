// 为窄屏纵向列表补充表头标签；仍复用同一份桌面数据与交互。
function labelTableCells(root) {
  root.querySelectorAll('.table-scroll table').forEach(table => {
    const labels = [...table.querySelectorAll('thead th')].map(th => th.textContent.trim());
    table.querySelectorAll('tbody tr').forEach(row => {
      [...row.cells].forEach((cell, index) => {
        if (cell.colSpan === 1 && !cell.dataset.label) cell.dataset.label = labels[index] || '';
      });
    });
  });
}

const api = async (path, opts = {}) => {
  const r = await fetch(path, { credentials: 'include', ...opts,
    headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) } });
  if (r.status === 401) { renderLogin(); throw new Error('unauthorized'); }
  return r.json();
};

const fmt = {
  ts: (sec) => new Date(sec * 1000).toLocaleString('zh-CN', { hour12: false }),
  ms: (ms) => ms == null ? '-' : (ms < 1000 ? ms + 'ms' : (ms/1000).toFixed(2) + 's'),
  num: (n) => (n || 0).toLocaleString(),
  pct: (n) => (n * 100).toFixed(1) + '%',
};

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(String(text ?? ''));
  } catch (error) {
    const input = document.createElement('textarea');
    const focused = document.activeElement;
    input.value = String(text ?? '');
    input.style.cssText = 'position:fixed;left:-9999px;top:0;';
    document.body.appendChild(input);
    try {
      input.select();
      if (!document.execCommand('copy')) throw new Error('clipboard unavailable');
    } finally {
      input.remove();
      if (focused && focused.isConnected) focused.focus();
    }
  }
}

function flash(msg) {
  let stack = document.getElementById('toastStack');
  if (!stack) {
    stack = document.createElement('div');
    stack.id = 'toastStack';
    stack.className = 'toast-stack';
    stack.setAttribute('aria-label', '操作通知');
    document.body.appendChild(stack);
  }
  const toast = document.createElement('div');
  toast.className = 'toast';
  toast.setAttribute('role', 'status');
  const text = String(msg ?? '');
  toast.innerHTML = `<div class="toast-message">${escapeHtml(text)}</div>
    <div class="toast-actions">
      ${text.length > 160 ? '<button type="button" data-details>查看详情</button>' : ''}
      <button type="button" data-close aria-label="关闭此通知">关闭</button>
    </div>`;
  toast.querySelector('[data-close]').onclick = () => {
    toast.remove();
    if (!stack.children.length) stack.remove();
  };
  const details = toast.querySelector('[data-details]');
  if (details) details.onclick = () => showErrorDetail(text);
  stack.appendChild(toast);
}
