async function loadProxiesPage() { const data = await api("api/proxies"); return renderProxyPool(data); }
function renderProxyPool(data) {
    return `
      <div class="panel page-fill">
        <h2>代理池
          <span class="controls"><button class="btn" onclick="addProxy()">+ 添加代理</button></span>
        </h2>
        <details class="note">
          <summary>每个代理是一个独立 IP slot</summary>
          <div class="body">
            每个代理享有独立的并发 / RPM / RPH 配额，添加更多代理可提升总容量。
            请求自动选择有空位的代理，全部占满时本地返回 HTTP 429。连续失败 5 次触发熔断，
            可在列表中「重置」。<br><br>
            配置代理后默认仅走代理池，避免出口占满时占用主机 IP；需要直连回退时，
            在「设置」页开启「无可用代理时退回直连」。熔断的代理默认停用
            120 分钟（历史测试约 106-121 分钟恢复），也可在此手动重置。
            单出口的可用容量受上游风控与出口质量影响，扩容通过增加出口实现。
          </div>
        </details>
        <div class="table-scroll">
          <table>
            <colgroup>
              <col style="width:56px"><col style="width:150px"><col>
              <col style="width:88px"><col style="width:80px"><col style="width:200px">
               <col style="width:236px">
            </colgroup>
            <thead><tr>
              <th class="nw">ID</th><th class="nw">名称</th><th>代理 URL</th><th class="nw">状态</th>
              <th class="nw">失败</th><th class="nw">最近错误</th><th class="nw">操作</th>
            </tr></thead>
            <tbody>${(data.items || []).map(p => `
              <tr>
                <td class="num nw">${p.id}</td>
                <td class="truncate" title="${escapeHtml(p.name)}">${escapeHtml(p.name)}</td>
                <td class="pre truncate" title="${escapeHtml(p.url)}">${escapeHtml(p.url)}</td>
                <td class="nw">${p.enabled ? '<span class="badge ok">已启用</span>' : '<span class="badge muted">已禁用</span>'}</td>
                <td class="num nw">${p.fail_count > 0 ? '<span class="badge fail">' + p.fail_count + '</span>' : p.fail_count}</td>
                 <td>${renderErrorSummary(p.last_error)}</td>
                <td class="nw">
                  <span class="actions">
                    <button class="btn muted" onclick="toggleProxy(${p.id})">${p.enabled ? '禁用' : '启用'}</button>
                    <button class="btn muted" onclick="resetProxy(${p.id})">重置</button>
                    <button class="btn danger" onclick="delProxy(${p.id})">删除</button>
                  </span>
                </td>
              </tr>`).join('') || '<tr><td colspan="7" class="empty">还没有代理。点「+ 添加代理」加一个。</td></tr>'}
            </tbody>
          </table>
        </div>
      </div>
    `;
}

async function addProxy() {
  const data = await modalForm({
    title: '添加代理',
    desc: '每个代理是一个独立 IP slot，享有独立的并发/RPM/RPH 配额。',
    fields: [
      { name: 'name', label: '名称', placeholder: 'vps-uk-1', required: true,
        hint: '随便起，admin 面板里用来区分' },
      { name: 'url', label: '代理 URL', placeholder: 'http://user:pass@1.2.3.4:8080', required: true,
        hint: '支持 http:// / https:// / socks5:// 协议' },
    ],
  });
  if (!data) return;
  const r = await fetch('api/proxies', {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: data.name, url: data.url, weight: 1 }),
  });
  const j = await r.json();
  if (j.error) return flash('添加失败：' + j.error);
  flash('代理已添加');
  await refreshPage();
}
async function delProxy(id) {
  const ok = await modalConfirm({
    title: '删除代理',
    desc: `确定删除代理 #${id}？此操作不可撤销。`,
    confirmText: '删除',
    danger: true,
  });
  if (!ok) return;
  await api('api/proxies/' + id, { method: 'DELETE' });
  flash('已删除');
  await refreshPage();
}
async function toggleProxy(id) {
  await api('api/proxies/' + id + '/toggle', { method: 'POST' });
  await refreshPage();
}
async function resetProxy(id) {
  await api('api/proxies/' + id + '/reset', { method: 'POST' });
  flash('已重置失败计数');
  await refreshPage();
}
