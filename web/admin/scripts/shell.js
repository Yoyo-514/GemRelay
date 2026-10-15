const pages = {
  dashboard: loadDashboardPage,
  requests: loadRequestsPage,
  proxies: loadProxiesPage,
  cookies: loadCookiesPage,
  settings: loadSettingsPage,
};

// ─── Routing / shell ────────────────────────────────────────────────────────

let currentPage = null;
let pageSeq = 0;

// 点已经选中的导航项直接忽略：重新拉一遍数据、整块内容闪一下再画回原样，
// 用户看到的只是抖动。要重新取数有「刷新」按钮，走 refreshPage()。
async function switchPage(name, force = false) {
  if (!force && name === currentPage) return;
  const seq = ++pageSeq;
  currentPage = name;

  document.querySelectorAll('nav button').forEach(button => {
    const selected = button.dataset.page === name;
    button.classList.toggle('active', selected);
    if (selected) button.setAttribute('aria-current', 'page');
    else button.removeAttribute('aria-current');
  });
  const titleEl = document.getElementById('pageTitle');
  if (titleEl) titleEl.textContent = PAGE_TITLES[name] || name;
  const main = document.getElementById('main');

  // 旧内容先留着，超过 200ms 还没好才显示占位——快的时候完全不闪
  const slow = setTimeout(() => {
    if (seq === pageSeq) main.innerHTML = '<div class="empty">加载中…</div>';
  }, 200);
  let html;
  try {
    html = await pages[name]();
  } catch (e) {
    clearTimeout(slow);
    if (seq === pageSeq) main.innerHTML = '<div class="empty" role="alert">页面加载失败：' + escapeHtml(e.message) + '</div>';
    return;
  }
  clearTimeout(slow);
  if (seq !== pageSeq) return;   // 期间又点了别的页，丢弃这次结果
  main.innerHTML = html;
  labelTableCells(main);

  if (name === 'dashboard') {
    await loadChart('24h');
    document.getElementById('rangeSel').onchange = (e) => loadChart(e.target.value);
    await populateTestProxies();
  }
  if (name === 'requests') {
    reqOffset = 0;
    document.getElementById('reqStatus').onchange = () => { reqOffset = 0; loadReqs(); };
    document.getElementById('reqModel').onchange = () => { reqOffset = 0; loadReqs(); };
    document.getElementById('reqSize').onchange = () => { reqOffset = 0; loadReqs(); };
    await loadReqs();
  }
  if (name === 'settings') {
    await loadRtCfg();
  }
  if (name === 'cookies') {
    await loadCookieQuotas();
  }
}

// 主动重新拉当前页的数据（「刷新」按钮、增删改之后回填）
function refreshPage() { return switchPage(currentPage, true); }

// 侧栏图标（Material Symbols 风格的简化路径，内联避免外部依赖）
const ICONS = {
  dashboard: '<path d="M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z"/>',
  requests: '<path d="M7 5h14v2H7V5zm0 6h14v2H7v-2zm0 6h14v2H7v-2zM3 4.5a1.5 1.5 0 113 0 1.5 1.5 0 01-3 0zm0 6a1.5 1.5 0 113 0 1.5 1.5 0 01-3 0zm0 6a1.5 1.5 0 113 0 1.5 1.5 0 01-3 0z"/>',
  proxies: '<path d="M12 2a2.5 2.5 0 011 4.79V9h4a2 2 0 012 2v1.21a2.5 2.5 0 11-2 0V11h-4v1.21a2.5 2.5 0 11-2 0V11H7v1.21a2.5 2.5 0 11-2 0V11a2 2 0 012-2h4V6.79A2.5 2.5 0 0112 2z"/>',
  settings: '<path d="M3 17v2h6v-2H3zM3 5v2h10V5H3zm10 16v-2h8v-2h-8v-2h-2v6h2zM7 9v2H3v2h4v2h2V9H7zm14 4v-2H11v2h10zm-6-4h2V7h4V5h-4V3h-2v6z"/>',
  cookies: '<path d="M12 2a10 10 0 100 20 10 10 0 000-20zm-1 4a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0zm7 3a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0zM8 13a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0zm6 4a1.5 1.5 0 11-3 0 1.5 1.5 0 013 0z"/>',
  menu: '<path d="M3 6h18v2H3V6zm0 5h18v2H3v-2zm0 5h18v2H3v-2z"/>',
  logout: '<path d="M17 7l-1.41 1.41L18.17 11H8v2h10.17l-2.58 2.58L17 17l5-5-5-5zM4 5h8V3H4a2 2 0 00-2 2v14a2 2 0 002 2h8v-2H4V5z"/>',
};
const NAV = [
  ['dashboard', '概览'],
  ['requests', '请求记录'],
  ['proxies', '代理池'],
  ['cookies', 'Cookie 池'],
  ['settings', '设置'],
];
const PAGE_TITLES = {
  dashboard: '概览', requests: '请求记录', proxies: '代理池', cookies: 'Cookie 池', settings: '设置',
};

function toggleSidebar() {
  const el = document.querySelector('.sidebar');
  el.classList.toggle('collapsed');
  try { localStorage.setItem('sbCollapsed', el.classList.contains('collapsed') ? '1' : ''); } catch (e) {}
}

function renderShellMarkup(collapsed) {
  return `
    <a class="skip-link" href="#main">跳到主要内容</a>
    <aside class="sidebar${collapsed ? ' collapsed' : ''}">
      <div class="side-top">
        <button class="icon-btn" onclick="toggleSidebar()" title="收起 / 展开" aria-label="收起或展开侧边栏">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">${ICONS.menu}</svg>
        </button>
        <span class="brand-text"><span class="spark"></span>GemRelay</span>
      </div>
      <nav aria-label="主要导航">
        ${NAV.map(([k, label]) => `
          <button data-page="${k}" onclick="switchPage('${k}')" title="${label}">
            <svg viewBox="0 0 24 24">${ICONS[k]}</svg>
            <span class="nav-label">${label}</span>
          </button>`).join('')}
      </nav>
      <div class="side-foot">
        <div id="sideStat"></div>
        <div class="sf-sep"></div>
        <button class="logout-btn" onclick="doLogout()" title="退出登录">
          <svg viewBox="0 0 24 24">${ICONS.logout}</svg>
          <span class="nav-label">退出登录</span>
        </button>
      </div>
    </aside>
    <div class="content">
      <div class="topbar">
        <h1 class="page-title" id="pageTitle">概览</h1>
        <div class="topbar-right">
          <span class="service-status" role="status">
            <span class="dot pending" id="healthDot" aria-hidden="true"></span>
            <span id="healthText">正在连接…</span>
          </span>
          <button class="icon-btn mobile-logout" onclick="doLogout()" aria-label="退出登录" title="退出登录">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">${ICONS.logout}</svg>
          </button>
        </div>
      </div>
      <main id="main" tabindex="-1"></main>
    </div>
  `;
}

function renderShell() {
  const collapsed = (() => { try { return localStorage.getItem('sbCollapsed') === '1'; } catch (e) { return false; } })();
  document.getElementById('app').innerHTML = renderShellMarkup(collapsed);
  switchPage('dashboard');
  loadSideStat();
}

// 侧栏底部放真正有用的服务状态，而不是一个占位破折号
async function loadSideStat() {
  const box = document.getElementById('sideStat');
  if (!box) return;
  try {
    const [root, ck, usage] = await Promise.all([
      fetch('../').then(r => r.json()).catch(() => null),
      api('api/cookies').catch(() => null),
      api('api/usage').catch(() => null),
    ]);
    const dot = document.getElementById('healthDot');
    const ht = document.getElementById('healthText');
    if (dot) { dot.classList.remove('pending'); dot.classList.toggle('bad', !root); }
    if (ht) ht.textContent = root ? '运行中' : '无法连接';
    box.innerHTML = [
      ['版本', root ? 'v' + root.version : '—'],
      ['模型', root ? root.models.length + ' 个' : '—'],
      ['Cookie', ck && ck.items && ck.items.length ? ck.items.length + ' 个' : '匿名'],
      ['IP slot', usage ? (usage.items || []).length + ' 个' : '—'],
    ].map(([k, v]) => `<div class="sf-row"><span>${k}</span><b>${v}</b></div>`).join('');
  } catch (e) { box.innerHTML = ''; }
}
