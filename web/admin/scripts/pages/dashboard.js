// 单出口突发上限的记录区间为 80-180 次，取下沿作为参考线。
// 实际可请求次数受连接复用、出口质量与请求节奏影响。
const BAN_LINE = 80;

let charts = {};
const destroyChart = (key) => { if (charts[key]) { charts[key].destroy(); delete charts[key]; } };

async function loadDashboardPage() {
    const stats = await api('api/stats?hours=24');
    const usage = await api('api/usage');
    return `
      <div class="kpi-row">
        <div class="kpi"><div class="label">24 小时请求</div><div class="value">${fmt.num(stats.total_requests)}</div></div>
        <div class="kpi"><div class="label">成功率</div><div class="value">${fmt.pct(stats.success_rate)}</div>
          <div class="sub">${fmt.num(stats.success)} 成功 · ${fmt.num(stats.failures)} 失败</div></div>
        <div class="kpi"><div class="label">平均延迟</div><div class="value">${fmt.ms(stats.avg_ms)}</div></div>
        <div class="kpi"><div class="label">输入 Token</div><div class="value">${fmt.num(stats.prompt_tokens)}</div></div>
        <div class="kpi"><div class="label">输出 Token</div><div class="value">${fmt.num(stats.output_tokens)}</div></div>
      </div>

      <div class="grid-main">
        <div class="panel">
          <h2>请求量与延迟
            <span class="controls">
              <select id="rangeSel">
                <option value="24h">最近 24 小时</option>
                <option value="7d">最近 7 天</option>
                <option value="30d">最近 30 天</option>
              </select>
            </span>
          </h2>
          <div class="chart-wrap"><canvas id="chartReq"></canvas></div>
        </div>

        <div class="stack">
        <div class="panel">
          <h2>距离封禁红线
            <span class="controls"><button onclick="refreshPage()">刷新</button></span>
          </h2>
          <div class="slot-scroll">
            ${(usage.items || []).map(u => {
              const rph = u.rph || 0;
              const limit = u.limit_rph || 0;
              const pct = Math.min(rph / BAN_LINE * 100, 100);
              const limitPct = limit ? Math.min(limit / BAN_LINE * 100, 100) : null;
              const left = Math.max(BAN_LINE - rph, 0);
              let noteCls = '', note = `还能再打约 ${left} 次`;
              if (rph >= BAN_LINE) { noteCls = 'danger'; note = '已越过红线，随时可能被拦'; }
              else if (rph >= BAN_LINE * 0.75) { noteCls = 'warn'; note = `接近红线，只剩约 ${left} 次`; }
              else if (rph === 0) { note = '本小时还没有请求'; }
              return `<div class="slot">
                <div class="slot-head">
                  <span class="slot-name">${escapeHtml(u.name)}</span>
                  <span class="slot-count"><b>${rph}</b> / ${BAN_LINE}</span>
                </div>
                <div class="track">
                  <div class="fill" style="width:${pct}%"></div>
                  ${limitPct !== null ? `<div class="redline" style="left:${limitPct}%"
                    title="本地 RPH 限额 ${limit}"></div>` : ''}
                </div>
                <div class="slot-note ${noteCls}">${note} · 并发 ${u.inflight}/${u.limit_concurrent || '∞'}
                  · RPM ${u.rpm}/${u.limit_rpm || '∞'} · RPH ${rph}/${u.limit_rph || '∞'}</div>
              </div>`;
            }).join('') || '<div class="empty">还没有任何请求经过</div>'}
          </div>
          <details class="note" style="margin:12px 0 0">
            <summary>这条线是怎么来的</summary>
            <div class="body">
              单个出口的突发上限记录为 <b>80-180 次</b>，轨道取下沿作为参考。
              区间宽度主要来自连接策略与出口质量：并发 10 时，复用连接池记录到 172/177 次，
              每次新建连接为 106/109 次，静态 IP 曾达到 188 次。<br><br>
              请求节奏同样影响明显：10 次/分钟的静态 IP 在 110 分钟内完成 800 次请求，
              均未触发拦截。轨道是保守参考，具体容量结合出口表现调整。<br><br>
              触发拦截后，前 20 分钟内的 60 次探测均失败，106-121 分钟恢复。
              红竖线是本地 RPH 限额的位置，超限时本地返回 429。
              扩大容量可在「代理池」添加代理，每个代理是独立 slot。
            </div>
          </details>
        </div>

        <div class="panel">
          <h2>连通性诊断
            <span class="controls"><button class="btn" onclick="runTest()">测试一次</button></span>
          </h2>
          <select id="testProxy" style="width:100%">
            <option value="">自动选择（按调度规则）</option>
            <option value="0">直连主机 IP</option>
          </select>
          <p class="hint" style="margin-top:10px">
            发送一次真实请求以检查连通性，不占用限流配额，也不写入请求记录。
            返回 302/429 说明出口受限，可更换 IP。
          </p>
          <div id="testResult" style="display:none; margin-top:12px;"></div>
        </div>
        </div>
      </div>

      <div class="grid-2">
        <div class="panel">
          <h2>按模型分组</h2>
          <table>
            <colgroup><col style="width:38%"><col style="width:14%"><col style="width:14%"><col style="width:16%"><col style="width:18%"></colgroup>
            <thead><tr><th>模型</th><th class="nw">请求</th><th class="nw">成功</th><th class="nw">平均</th><th class="nw">Token</th></tr></thead>
            <tbody>${(stats.by_model || []).map(m => `
              <tr><td class="truncate" title="${escapeHtml(m.model)}">${m.model}</td>
                  <td class="num nw">${fmt.num(m.requests)}</td>
                  <td class="num nw">${fmt.num(m.success)}</td>
                  <td class="num nw">${fmt.ms(m.avg_ms)}</td>
                  <td class="num nw">${fmt.num(m.prompt_tokens + m.output_tokens)}</td></tr>`).join('')
              || '<tr><td colspan="5" class="empty">暂无数据</td></tr>'}
            </tbody>
          </table>
        </div>
        <div class="panel">
          <h2>按出口分组</h2>
          <table>
            <colgroup><col style="width:46%"><col style="width:18%"><col style="width:18%"><col style="width:18%"></colgroup>
            <thead><tr><th>出口</th><th class="nw">请求</th><th class="nw">成功</th><th class="nw">平均</th></tr></thead>
            <tbody>${(stats.by_proxy || []).map(p => `
              <tr><td class="truncate">${!p.proxy || p.proxy === '(direct)' ? '直连' : escapeHtml(p.proxy)}</td>
                  <td class="num nw">${fmt.num(p.requests)}</td>
                  <td class="num nw">${fmt.num(p.success)}</td>
                  <td class="num nw">${fmt.ms(p.avg_ms)}</td></tr>`).join('')
              || '<tr><td colspan="4" class="empty">暂无数据</td></tr>'}
            </tbody>
          </table>
        </div>
      </div>

      <div class="panel">
          <h2>错误分类（近 ${stats.window_hours || 24}h）</h2>
          <table>
            <colgroup><col style="width:22%"><col style="width:10%"><col style="width:68%"></colgroup>
            <thead><tr><th>类型</th><th class="nw">次数</th><th>含义 / 该怎么办</th></tr></thead>
            <tbody>
              ${(stats.error_breakdown || []).map(e => `<tr>
                <td class="nw"><span class="badge ${ERR_BADGE[e.kind] || 'muted'}">${escapeHtml(e.kind)}</span></td>
                <td class="num nw">${e.count}</td>
                <td style="font-size:12.5px">${ERR_DESC[e.kind] || ''}
                  <div class="pre" style="margin-top:5px;color:var(--muted)">${escapeHtml(e.sample || '')}</div></td>
              </tr>`).join('') || '<tr><td colspan="3" class="empty">窗口内没有失败</td></tr>'}
            </tbody>
          </table>
      </div>
    `;
  }
async function loadChart(range = '24h') {
  destroyChart('req');
  const data = await api('api/timeseries?range=' + range);
  const labels = (data.series || []).map(s => fmt.ts(s.bucket));
  const reqs = (data.series || []).map(s => s.requests);
  const p50 = (data.series || []).map(s => s.p50_ms);
  const ctx = document.getElementById('chartReq');
  if (!ctx) return;
  const g = ctx.getContext('2d');
  const areaBlue = g.createLinearGradient(0, 0, 0, 260);
  areaBlue.addColorStop(0, 'rgba(66,133,244,.22)');
  areaBlue.addColorStop(1, 'rgba(66,133,244,0)');
  charts.req = new Chart(ctx, {
    type: 'line',
    data: {
      labels,
      datasets: [
        { label: '请求数', data: reqs, yAxisID: 'y',
          borderColor: '#0B57D0', backgroundColor: areaBlue, fill: true,
          tension: .35, borderWidth: 2, pointRadius: 0, pointHoverRadius: 4,
          pointBackgroundColor: '#0B57D0' },
        { label: 'P50 延迟', data: p50, yAxisID: 'y1',
          borderColor: '#9B72CB', backgroundColor: 'transparent',
          tension: .35, borderWidth: 2, borderDash: [5, 4],
          pointRadius: 0, pointHoverRadius: 4, pointBackgroundColor: '#9B72CB' },
      ],
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      layout: { padding: { top: 4 } },
      plugins: {
        legend: {
          align: 'end',
          labels: { color: '#5F6368', usePointStyle: true, pointStyle: 'circle',
                    boxWidth: 8, boxHeight: 8, padding: 16,
                    font: { size: 12, family: 'inherit' } },
        },
        tooltip: {
          backgroundColor: '#1F1F1F', padding: 10, cornerRadius: 8,
          titleFont: { size: 12, weight: '500' }, bodyFont: { size: 12 },
          displayColors: true, usePointStyle: true, boxPadding: 4,
        },
      },
      scales: {
        x: { ticks: { color: '#5F6368', maxRotation: 0, autoSkipPadding: 24,
                      font: { size: 11 } },
             grid: { display: false }, border: { color: '#E8ECF1' } },
        y: { type: 'linear', position: 'left', beginAtZero: true,
             ticks: { color: '#5F6368', font: { size: 11 }, precision: 0, maxTicksLimit: 6 },
             grid: { color: '#EDF1F7' }, border: { display: false } },
        y1: { type: 'linear', position: 'right', beginAtZero: true,
              ticks: { color: '#5F6368', font: { size: 11 }, maxTicksLimit: 6,
                       callback: (v) => v + 'ms' },
              grid: { drawOnChartArea: false }, border: { display: false } },
      },
    },
  });
}

// 连通性诊断 ──────────────────────────────────────────────────────────────

async function populateTestProxies() {
  const sel = document.getElementById('testProxy');
  if (!sel) return;
  // 清掉旧的代理选项（保留前两个固定项）
  while (sel.options.length > 2) sel.remove(2);
  try {
    const data = await api('api/proxies');
    (data.items || []).filter(p => p.enabled).forEach(p => {
      const opt = document.createElement('option');
      opt.value = p.id;
      opt.text = '代理: ' + p.name;
      sel.appendChild(opt);
    });
  } catch (e) { /* ignore — login redirect handled in api() */ }
}

async function runTest() {
  const sel = document.getElementById('testProxy');
  const proxy = sel.value;
  const box = document.getElementById('testResult');
  box.style.display = 'block';
  box.innerHTML = '<div class="empty">正在测试…（最多等 30 秒）</div>';
  try {
    const url = 'api/test' + (proxy !== '' ? '?proxy_id=' + proxy : '');
    const r = await api(url);
    const statusBadge = {
      success: '<span class="badge ok">成功</span>',
      blocked_sorry: '<span class="badge fail">被风控（302）</span>',
      rate_limited: '<span class="badge fail">被限流（429）</span>',
      upstream_error: '<span class="badge fail">上游错误</span>',
      network_error: '<span class="badge fail">网络层错误</span>',
    }[r.status] || `<span class="badge muted">${r.status}</span>`;
    box.innerHTML = `
      <div style="display: grid; grid-template-columns: max-content 1fr; gap: 8px 16px; font-size: 13px;">
        <div style="color: var(--muted)">结果</div><div>${statusBadge}</div>
        <div style="color: var(--muted)">HTTP code</div><div><code>${r.http_code || '-'}</code></div>
        <div style="color: var(--muted)">总用时</div><div><code>${r.total_ms} ms</code></div>
        <div style="color: var(--muted)">使用的 IP slot</div><div>${r.proxy_name || '-'}${r.use_direct ? '' : (r.proxy_id ? ' (代理 #' + r.proxy_id + ')' : '')}</div>
        <div style="color: var(--muted)">指纹</div><div><code>${r.impersonate}</code></div>
        ${r.response_text ? `<div style="color: var(--muted)">回复内容</div><div class="pre">${escapeHtml(r.response_text)}</div>` : ''}
        <div style="color: var(--muted)">诊断说明</div><div style="line-height: 1.6;">${escapeHtml(r.diagnostic)}</div>
        ${r.upstream_snip && !r.ok ? `<div style="color: var(--muted)">上游响应片段</div>
          <div class="pre" style="max-height: 120px; overflow: auto; background: var(--bg); padding: 8px; border-radius: 4px;">${escapeHtml(r.upstream_snip)}</div>` : ''}
      </div>
    `;
  } catch (e) {
    box.innerHTML = `<div class="err">测试请求失败：${e.message}</div>`;
  }
}
