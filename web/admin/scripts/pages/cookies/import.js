// 逐项模式列出的 cookie。顺序按从 DevTools 中查找的难易排列。
// req 只标 SAPISID：后端仅强制这一项（用于计算 SAPISIDHASH 授权头），
// 其余项缺失时账号仍可能可用，统一标为必填会阻挡可用账号。
const COOKIE_KEYS = [
  { k: '__Secure-1PSID', hint: '主会话' },
  { k: 'SAPISID', req: true, hint: '算授权头用，缺了会被当匿名' },
  { k: '__Secure-1PSIDTS', hint: '约 30 分钟过期，保活会自动换发' },
  { k: 'SID' }, { k: 'HSID' }, { k: 'SSID' }, { k: 'APISID' },
];

// 把 "k=v; k=v" 拆成对象。值里可能含 '='（base64 补位），所以只在第一个等号处切。
function parseCookieStr(s) {
  const out = {};
  (s || '').split(';').forEach(p => {
    const t = p.trim();
    const i = t.indexOf('=');
    if (i > 0) out[t.slice(0, i)] = t.slice(i + 1);
  });
  return out;
}

function joinCookieObj(obj) {
  return Object.entries(obj).filter(([, v]) => (v || '').trim())
    .map(([k, v]) => `${k}=${String(v).trim()}`).join('; ');
}

async function addCookieAcct() {
  // 两个模式共用这一份状态，切 tab 时互相带过去。extras 存的是不在
  // COOKIE_KEYS 里的项——整串粘贴进来的 cookie 往往还带一堆别的，
  // 切到逐项再切回去不能把它们弄丢。
  let extras = {};

  const data = await buildModal({
    title: '导入 Cookie',
    wide: true,
    desc: '浏览器 F12 → Application → Cookies → 选 <code>https://gemini.google.com</code>。逐项抄或整串粘贴都行，两种填法可随时切换，已填的内容会带过去。',
    bodyHTML: `
      <div class="field">
        <label>标签</label>
        <input data-name="label" type="text" placeholder="例如账号邮箱，可留空" />
        <div class="hint">仅用于面板里区分账号，不参与请求。</div>
      </div>

      <div class="segbar" data-role="seg">
        <button type="button" data-mode="raw" class="on">整串粘贴（推荐）</button>
        <button type="button" data-mode="fields">逐项填写</button>
      </div>

      <div data-pane="fields" style="display:none;">
        <div class="ck-grid">
          ${COOKIE_KEYS.map(c => `
            <div class="field${c.k.length > 12 ? ' wide' : ''}">
              <label>${escapeHtml(c.k)}${c.req ? ' <span class="req">*</span>' : ''}</label>
              <input data-ck="${escapeHtml(c.k)}" type="text" autocomplete="off" spellcheck="false"
                     placeholder="${escapeHtml(c.hint || '')}" />
            </div>`).join('')}
        </div>
        <div class="ck-extra" data-role="extras"></div>
        <div class="ck-extra">前三项构成最小可用集合，下面四项用于补齐。
        浏览器请求实际会携带 <b>20 多个</b> cookie，服务端将整串原样转发
        （仅解析 SAPISID 用于计算授权头），
        <b>整串粘贴更接近真实浏览器</b>；逐项模式适合手头只有部分条目的情况。</div>
      </div>

      <div data-pane="raw">
        <div class="field">
          <textarea data-name="cookie" rows="6" spellcheck="false"
            placeholder="SID=...; HSID=...; SSID=...; APISID=...; SAPISID=...; __Secure-1PSID=...; __Secure-1PSIDTS=..."></textarea>
          <div class="hint">整行粘贴，分号分隔，至少包含 SAPISID 和 __Secure-1PSID。
          建议从 Firefox 导出：其会话可通过服务保活续期，新版 Chrome 的会话绑定设备，续期可能失败。
          DevTools 中全选复制即可，多余的项会一并保留。</div>
        </div>
      </div>

      <div class="field" style="margin-top:14px;">
        <label>备注</label>
        <input data-name="note" type="text" placeholder="可留空" />
      </div>`,
    submitText: '导入',
    onReady: (overlay) => {
      const $ = (s) => overlay.querySelector(s);
      const inputs = () => [...overlay.querySelectorAll('[data-ck]')];
      const ta = $('[data-name=cookie]');

      const fieldsToRaw = () => {
        const obj = {};
        inputs().forEach(el => { if (el.value.trim()) obj[el.dataset.ck] = el.value.trim(); });
        ta.value = joinCookieObj({ ...obj, ...extras });
      };
      const rawToFields = () => {
        const obj = parseCookieStr(ta.value);
        const known = new Set(COOKIE_KEYS.map(c => c.k));
        extras = {};
        Object.entries(obj).forEach(([k, v]) => { if (!known.has(k)) extras[k] = v; });
        inputs().forEach(el => { el.value = obj[el.dataset.ck] || ''; });
        const n = Object.keys(extras).length;
        $('[data-role=extras]').textContent = n
          ? `另有 ${n} 项未在上面列出（${Object.keys(extras).slice(0, 4).join(', ')}${n > 4 ? ' …' : ''}），会一并保留。`
          : '';
      };

      overlay.querySelectorAll('[data-role=seg] button').forEach(btn => {
        btn.onclick = () => {
          const mode = btn.dataset.mode;
          if (mode === 'raw') fieldsToRaw(); else rawToFields();
          overlay.querySelectorAll('[data-role=seg] button')
            .forEach(b => b.classList.toggle('on', b === btn));
          $('[data-pane=fields]').style.display = mode === 'fields' ? '' : 'none';
          $('[data-pane=raw]').style.display = mode === 'raw' ? '' : 'none';
        };
      });
      // 在逐项模式里直接粘贴一整串时自动拆开，省得用户先去切 tab
      inputs().forEach(el => el.addEventListener('paste', (ev) => {
        const txt = (ev.clipboardData || window.clipboardData).getData('text') || '';
        if (!txt.includes(';') || !txt.includes('=')) return;
        ev.preventDefault();
        ta.value = txt.trim();
        rawToFields();
      }));
    },
    onSubmit: (overlay) => {
      const label = (overlay.querySelector('[data-name=label]').value || '').trim();
      const note = (overlay.querySelector('[data-name=note]').value || '').trim();
      const isRaw = overlay.querySelector('[data-role=seg] button.on').dataset.mode === 'raw';
      let cookie;
      if (isRaw) {
        cookie = (overlay.querySelector('[data-name=cookie]').value || '').trim();
      } else {
        const obj = {};
        overlay.querySelectorAll('[data-ck]').forEach(el => {
          if (el.value.trim()) obj[el.dataset.ck] = el.value.trim();
        });
        cookie = joinCookieObj(obj);
      }
      if (!cookie) return { error: 'Cookie 不能为空' };
      if (!cookie.includes('SAPISID')) return { error: 'Cookie 缺少 SAPISID，请检查是否复制完整' };
      return { label, cookie, note };
    },
  });
  if (!data) return;
  const r = await fetch('api/cookies', {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  const j = await r.json();
  if (j.error) return flash('导入失败：' + j.error);
  flash('Cookie 已导入 #' + j.id);
  await refreshPage();
}
