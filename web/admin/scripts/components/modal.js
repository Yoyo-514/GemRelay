// 自定义弹窗 ──────────────────────────────────────────────────────────────
//   modalConfirm({ title, desc, confirmText, danger })       -> bool
//   modalPrompt({ title, desc, label, placeholder, value })   -> string|null
//   modalForm({ title, desc, fields: [{name,label,...}], submitText })
//                                                            -> obj|null

function buildModal({ title, desc, bodyHTML, submitText, cancelText = '取消', danger, wide, allowEnter, dismissOnBackdrop = true, onReady, onSubmit }) {
  return new Promise(resolve => {
    const previousFocus = document.activeElement;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
      <div class="modal${wide ? ' wide' : ''}" role="dialog" aria-modal="true" aria-label="${escapeHtml(title || '操作确认')}" tabindex="-1">
        ${title ? `<h3>${escapeHtml(title)}</h3>` : ''}
        ${desc ? `<div class="desc">${desc}</div>` : ''}
        <div class="modal-err" role="alert" style="display:none;"></div>
        <div class="modal-body">${bodyHTML || ''}</div>
        <div class="actions">
          <button class="btn muted" data-act="cancel">${escapeHtml(cancelText)}</button>
          <button class="btn ${danger ? 'danger' : ''}" data-act="ok">${escapeHtml(submitText || '确定')}</button>
        </div>
      </div>`;
    document.body.appendChild(overlay);
    // 弹窗挂上 DOM 之后再交给调用方接事件——内容是 innerHTML 灌进去的，
    // 早于这一步拿不到里面的节点。
    if (onReady) onReady(overlay);

    const finish = (val) => {
      overlay.remove();
      document.removeEventListener('keydown', keyHandler);
      if (previousFocus && previousFocus.isConnected) previousFocus.focus();
      resolve(val);
    };
    const showErr = (msg) => {
      const e = overlay.querySelector('.modal-err');
      e.textContent = msg;
      e.style.display = msg ? 'block' : 'none';
      e.className = 'modal-err err';
    };

    overlay.querySelector('[data-act=cancel]').onclick = () => finish(null);
    overlay.querySelector('[data-act=ok]').onclick = async () => {
      const result = onSubmit ? await onSubmit(overlay) : true;
      if (result === false) return; // validation failed
      if (result && typeof result === 'object' && result.error) { showErr(result.error); return; }
      finish(result === true ? true : result);
    };
    overlay.onclick = (e) => { if (dismissOnBackdrop && e.target === overlay) finish(null); };

    const keyHandler = (e) => {
      if (e.key === 'Escape') { e.preventDefault(); finish(null); }
      if (e.key === 'Tab') {
        const targets = [...overlay.querySelectorAll('button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), a[href]')]
          .filter(el => el.getClientRects().length > 0);
        const first = targets[0], last = targets[targets.length - 1];
        if (!first) { e.preventDefault(); overlay.querySelector('.modal').focus(); }
        else if (e.shiftKey && (document.activeElement === first || !overlay.contains(document.activeElement))) {
          e.preventDefault(); last.focus();
        } else if (!e.shiftKey && (document.activeElement === last || !overlay.contains(document.activeElement))) {
          e.preventDefault(); first.focus();
        }
      }
      if (e.key === 'Enter' && allowEnter && document.activeElement.tagName !== 'TEXTAREA') {
        overlay.querySelector('[data-act=ok]').click();
      }
    };
    document.addEventListener('keydown', keyHandler);

    // 自动 focus 第一个 input
    setTimeout(() => {
      const first = overlay.querySelector('input,textarea');
      if (overlay.isConnected) (first || overlay.querySelector('[data-act=cancel]')).focus();
    }, 50);
  });
}

function modalConfirm({ title, desc, confirmText = '确定', danger = false }) {
  return buildModal({
    title, desc, bodyHTML: '', submitText: confirmText, danger, allowEnter: true,
    onSubmit: () => true,
  });
}

function modalPrompt({ title, desc, label, placeholder = '', value = '', hint, password = false }) {
  return buildModal({
    title, desc, allowEnter: true,
    bodyHTML: `<div class="field">
      ${label ? `<label>${escapeHtml(label)}</label>` : ''}
      <input type="${password ? 'password' : 'text'}" placeholder="${escapeHtml(placeholder)}" value="${escapeHtml(value)}" />
      ${hint ? `<div class="hint">${hint}</div>` : ''}
    </div>`,
    submitText: '确定',
    onSubmit: (overlay) => {
      const v = overlay.querySelector('input').value;
      if (!v) return { error: '不能为空' };
      return v;
    },
  });
}

// fields: [{ name, label, placeholder, hint, value, required, password }]
function modalForm({ title, desc, fields, submitText = '提交' }) {
  const html = fields.map(f => `
    <div class="field">
      <label>${escapeHtml(f.label)}${f.required ? ' *' : ''}</label>
      <input data-name="${escapeHtml(f.name)}" type="${f.password ? 'password' : 'text'}"
             placeholder="${escapeHtml(f.placeholder || '')}"
             value="${escapeHtml(f.value || '')}" />
      ${f.hint ? `<div class="hint">${f.hint}</div>` : ''}
    </div>`).join('');
  return buildModal({
    title, desc, bodyHTML: html, submitText, allowEnter: true,
    onSubmit: (overlay) => {
      const out = {};
      for (const f of fields) {
        const el = overlay.querySelector(`[data-name="${f.name}"]`);
        const v = (el.value || '').trim();
        if (f.required && !v) return { error: `「${f.label}」不能为空` };
        if (f.minLength && v.length < f.minLength) return { error: `「${f.label}」至少 ${f.minLength} 字符` };
        out[f.name] = v;
      }
      return out;
    },
  });
}
