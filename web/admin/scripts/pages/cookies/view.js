async function loadCookiesPage() { const data = await api("api/cookies"); return renderCookiePool(data); }
function renderCookiePool(data) {
    const items = data.items || [];
    const deadN = items.filter(a => a.health === 'dead').length;
    return `
      <div class="panel page-fill">
        <h2>Cookie 池
          <span class="controls">
            ${deadN > 0 ? '<span class="badge fail" style="margin-right:8px" title="这些号鉴权在失败，多半 cookie 过期了，建议用 Firefox 重新导出">' + deadN + ' 疑似失效</span>' : ''}
            <span class="badge ${data.enabled > 0 ? 'ok' : 'muted'}" style="margin-right:8px">${data.enabled || 0} 启用 / ${data.total || 0} 共</span>
            <button class="btn" onclick="addCookieAcct()">+ 导入 Cookie</button>
          </span>
        </h2>
        <details class="note">
          <summary>每行一个 Google 登录态账号</summary>
          <div class="body">
            粘贴 gemini.google.com 下的<b>完整 cookie 串</b>（至少含 SID / HSID / SSID /
            APISID / SAPISID / __Secure-1PSID / __Secure-1PSIDTS）。请求时按<b>最久未用优先</b>自动轮转挑一个
            启用的账号；模型权限和媒体生成额度由上游账号决定。<br><br>
            凭证保存后<b>不再回传浏览器</b>，列表只显示脱敏摘要（cookie 数 / 关键项 /
            SAPISID 末 4 位 / 失败次数）。缺 <code>__Secure-1PSIDTS</code> 的号会标黄，
            首次保活会尝试向 Google 补发；若反复失败请用 <b>Firefox</b> 重新导出
            （Chrome 新版本的设备绑定会话续不了）。
          </div>
        </details>
        <div class="table-scroll">
           <table class="accounts-table">
             <colgroup>
               <col style="width:48px"><col style="width:160px"><col style="width:80px">
               <col style="width:150px"><col style="width:92px"><col style="width:64px">
              <col style="width:220px"><col style="width:150px"><col><col style="width:270px">
            </colgroup>
            <thead><tr>
              <th class="nw">ID</th><th class="nw">标签</th><th class="nw">状态</th>
              <th class="nw">关键 Cookie</th><th class="nw">SAPISID</th><th class="nw">失败</th>
              <th class="nw">用量额度</th><th class="nw">最近成功</th><th>最近错误</th><th class="nw">操作</th>
            </tr></thead>
            <tbody>${items.map(a => `
              <tr class="${a.health === 'dead' ? 'row-dead' : ''}">
                <td class="num nw" data-label="ID">${a.id}</td>
                <td class="truncate" data-label="标签" title="${escapeHtml(a.label || '')}">${a.label ? escapeHtml(a.label) : '<span class="badge muted">未命名</span>'}</td>
                <td class="nw" data-label="状态">${
                  a.health === 'dead' ? '<span class="badge fail" title="鉴权在失败，多半 cookie 过期了，建议用 Firefox 重新导出">疑似失效</span>'
                  : a.health === 'disabled' ? '<span class="badge muted">禁用</span>'
                  : a.health === 'unknown' ? '<span class="badge warn" title="还没成功用过，点「检测」确认">待检测</span>'
                  : '<span class="badge ok">正常</span>'
                }</td>
                <td class="nw" data-label="关键 Cookie" title="共 ${a.cookie_count} 个 cookie，关键项：${(a.key_cookies||[]).join(' / ')}">
                   <span class="badge ${a.has_1psidts ? 'ok' : 'warn'}">${(a.key_cookies||[]).length}/9 项</span>
                  ${a.has_1psidts ? '' : '<span class="badge warn" title="缺 __Secure-1PSIDTS，首次保活会尝试补发；失败请用 Firefox 重新导出">缺 TS</span>'}
                </td>
                <td class="pre nw" data-label="SAPISID">${a.sapisid_tail ? '…' + escapeHtml(a.sapisid_tail) : '—'}</td>
                <td class="num nw" data-label="失败">${a.fail_count > 0 ? '<span class="badge fail">' + a.fail_count + '</span>' : 0}</td>
                <td class="account-quota" data-label="用量额度"><div class="quota-cell" data-quota-id="${a.id}" role="status" aria-live="polite" aria-label="账号 ${a.id} 用量额度">
                  <span class="quota-meta">等待查询…</span>
                </div></td>
                <td class="nw" data-label="最近成功" style="font-size:12px;color:var(--muted)">${a.last_ok_at ? fmt.ts(a.last_ok_at) : '—'}</td>
                 <td data-label="最近错误">${renderErrorSummary(a.last_error)}</td>
                <td class="nw account-actions" data-label="操作">
                  <span class="actions">
                    <button class="btn muted" onclick="checkCookieAcct(${a.id}, this)">检测</button>
                    <button class="btn muted" onclick="rotateCookieAcct(${a.id}, this)">保活</button>
                    <button class="btn muted" onclick="toggleCookieAcct(${a.id})">${a.status === 'enabled' ? '禁用' : '启用'}</button>
                    <button class="btn muted" onclick="editCookieAcct(${a.id}, ${JSON.stringify(a.label || '').replace(/"/g, '&quot;')}, ${JSON.stringify(a.note || '').replace(/"/g, '&quot;')})">编辑</button>
                    <button class="btn danger" onclick="delCookieAcct(${a.id})">删除</button>
                  </span>
                </td>
              </tr>`).join('') || '<tr><td colspan="10" class="empty">Cookie 池是空的。点「+ 导入 Cookie」加一个登录态账号。</td></tr>'}
            </tbody>
          </table>
        </div>
      </div>
    `;
}
