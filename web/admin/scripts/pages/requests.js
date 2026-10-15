async function loadRequestsPage() {
    return `
      <div class="panel page-fill">
        <h2>请求记录
          <span class="controls">
            <select id="reqStatus">
              <option value="">全部</option>
              <option value="ok">成功</option>
              <option value="fail">失败</option>
            </select>
            <select id="reqModel"><option value="">全部模型</option></select>
            <select id="reqSize">
              <option value="25">每页 25</option>
              <option value="50" selected>每页 50</option>
              <option value="100">每页 100</option>
              <option value="200">每页 200</option>
            </select>
            <button onclick="loadReqs()">刷新</button>
          </span>
        </h2>
        <div class="table-scroll">
          <table id="reqTable">
            <colgroup>
              <col style="width:140px"><col style="width:150px"><col style="width:130px">
              <col style="width:96px"><col style="width:70px"><col style="width:84px">
              <col style="width:84px"><col style="width:110px"><col style="width:64px">
              <col style="width:64px"><col>
            </colgroup>
            <thead><tr>
              <th class="nw">时间</th><th class="nw">模型</th><th class="nw">实际模型</th>
              <th class="nw">出口</th><th class="nw">账号</th><th class="nw">状态</th>
              <th class="nw">首字</th><th class="nw">总用时</th>
              <th class="nw">输入</th><th class="nw">输出</th><th>接口</th>
            </tr></thead>
            <tbody><tr><td colspan="11" class="empty">加载中...</td></tr></tbody>
          </table>
        </div>
        <div class="pager" id="reqPager"></div>
      </div>
    `;
  }
let reqOffset = 0;
// 每页条数由「每页 N」下拉决定；页面还没渲染时退回 50
function reqPageSize() {
  const el = document.getElementById('reqSize');
  return el ? parseInt(el.value, 10) : 50;
}
async function loadReqs() {
  const status = document.getElementById('reqStatus')?.value || '';
  const model = document.getElementById('reqModel')?.value || '';
  const url = `api/requests?limit=${reqPageSize()}&offset=${reqOffset}` +
              (status ? '&status=' + status : '') +
              (model ? '&model=' + encodeURIComponent(model) : '');
  const data = await api(url);
  const tbody = document.querySelector('#reqTable tbody');
  if (!tbody) return;
  tbody.innerHTML = (data.items || []).map(r => `
    <tr>
      <td class="nw muted">${fmt.ts(r.ts)}</td>
      <td class="truncate" title="${escapeHtml(r.model)}">${escapeHtml(r.model)}</td>
      <td class="nw">${upstreamCell(r)}</td>
      <td class="nw truncate">${r.proxy_name ? escapeHtml(r.proxy_name) : '<span class="badge muted">直连</span>'}</td>
      <td class="nw truncate" title="${escapeHtml(r.account_label || '')}">${
        r.account_id ? escapeHtml(r.account_label || ('#' + r.account_id))
                     : '<span class="badge muted">匿名</span>'}</td>
      <td class="nw">${r.status === 200 ? '<span class="badge ok">200</span>' : '<span class="badge fail">' + r.status + '</span>'}</td>
      <td class="num nw">${fmt.ms(r.ttfb_ms)}</td>
      <td class="num nw">${fmt.ms(r.total_ms)}</td>
      <td class="num nw">${fmt.num(r.prompt_tokens)}</td>
      <td class="num nw">${fmt.num(r.output_tokens)}</td>
      <td><span class="pre">${escapeHtml(r.endpoint)}${r.stream ? ' <span class="badge muted">stream</span>' : ''}</span>
         ${r.error ? renderErrorSummary(r.error) : ''}
      </td>
    </tr>`).join('') || '<tr><td colspan="11" class="empty">还没有请求记录</td></tr>';
  labelTableCells(tbody.closest('table').parentElement);
  const size = reqPageSize();
  const total = data.total || 0;
  const page = Math.floor(reqOffset / size) + 1;
  const pages = Math.max(Math.ceil(total / size), 1);
  const from = total ? reqOffset + 1 : 0;
  const to = Math.min(reqOffset + size, total);
  document.getElementById('reqPager').innerHTML = `
    <span class="muted">${from}-${to} / 共 ${total} 条</span>
    <span class="pager-btns">
      <button class="btn muted" ${reqOffset === 0 ? 'disabled' : ''}
        onclick="reqOffset=0;loadReqs()">最前</button>
      <button class="btn muted" ${reqOffset === 0 ? 'disabled' : ''}
        onclick="reqOffset=Math.max(0,reqOffset-${size});loadReqs()">上一页</button>
      <span class="muted" style="min-width:80px;text-align:center">第 ${page} / ${pages} 页</span>
      <button class="btn muted" ${to >= total ? 'disabled' : ''}
        onclick="reqOffset+=${size};loadReqs()">下一页</button>
      <button class="btn muted" ${to >= total ? 'disabled' : ''}
        onclick="reqOffset=${(pages-1)*size};loadReqs()">最后</button>
    </span>
  `;
  // populate model dropdown from current page (best-effort)
  const sel = document.getElementById('reqModel');
  if (sel && sel.options.length === 1) {
    const seen = new Set();
    (data.items || []).forEach(r => seen.add(r.model));
    [...seen].forEach(m => {
      const opt = document.createElement('option');
      opt.value = m; opt.text = m; sel.appendChild(opt);
    });
  }
}

// 服务端会静默降级（例如请求 3.1 Pro 实际给的是 3.6 Flash），只看请求名发现不了。
// upstream_model 取自响应帧里服务端自报的显示名；跟请求的对不上时标黄。
//
// 媒体模型例外：生图/音乐的后端名（Nano Banana / Lyria）跟请求名本来就不同，是预期
// 映射不是降级——真失败会直接报错不落 200 记录，所以这里把它们当"一致"，不误标黄。
const upstreamExpected = { image: 'nanobanana', music: 'lyria' };
function upstreamCell(r) {
  const up = r.upstream_model || '';
  if (!up) return '<span class="badge muted">—</span>';
  const norm = t => (t || '').toLowerCase().replace(/[\s._-]/g, '').replace(/^gemini/, '');
  const nm = norm(r.model), nu = norm(up);
  const same = nu.startsWith(nm.slice(0, 6)) ||
               (upstreamExpected[nm] && nu.startsWith(upstreamExpected[nm]));
  return same
    ? escapeHtml(up)
    : '<span class="badge warn" title="服务端实际使用的模型与请求的不一致">'
      + escapeHtml(up) + '</span>';
}
