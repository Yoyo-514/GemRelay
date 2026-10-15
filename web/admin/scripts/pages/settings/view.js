async function loadSettingsPage() { const k = await api("api/apikey"); return renderSettingsPage(k); }
function renderSettingsPage(k) {
    return `
       <div class="grid-main settings-layout">
         <div class="panel">
          <h2>运行时配置</h2>
          <p class="lede">
            改完点保存<b>立刻生效</b>，不用重启。存在数据库里，优先级高于
            <code>config.json</code> 和命令行参数。
          </p>
          <div id="rtCfgForm"></div>
          <div class="cfg-actions">
             <button class="btn primary" id="rtCfgSave" onclick="saveRtCfg()">保存并生效</button>
             <button class="btn muted" id="rtCfgReset" onclick="loadRtCfg()">放弃修改</button>
             <span id="rtCfgMsg" role="status" aria-live="polite"></span>
          </div>
        </div>

        <div class="stack">
          <div class="card">
            <h2>API Key</h2>
            <p class="lede">
              ${k.locked
                ? '已被 <code>--api-key</code> / <code>API_KEY</code> 锁定，改部署配置才能换。'
                : '给 <code>/v1/*</code> 接口鉴权，可随时轮换。'}
            </p>
            <div class="row" style="margin-bottom:10px;">
              <input id="apiKeyView" type="password" value="${escapeHtml(k.key)}"
                     style="flex:1; min-width:0; font-family:ui-monospace,monospace;" readonly />
              <button class="btn muted" onclick="toggleKeyVis()">显示</button>
              <button class="btn muted" onclick="copyKey()">复制</button>
            </div>
            ${k.locked ? '' : `
              <div class="row" style="margin-bottom:10px;">
                <button class="btn muted" onclick="rotateKey()">生成随机 Key</button>
                <button class="btn muted" onclick="customKey()">自定义</button>
              </div>`}
            <details class="note">
              <summary>调用示例</summary>
              <div class="body">
                <pre class="pre" style="margin:0; font-size:11.5px;">curl http://your-host/v1/chat/completions \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gemini-3.6-flash",
       "messages":[{"role":"user","content":"你好"}]}'</pre>
              </div>
            </details>
          </div>

          <div class="card">
            <h2>部署配置</h2>
            <p class="lede">
              改这些本来就要重启进程，所以放在 <code>docker-compose.yml</code> 里管，
              不做成网页表单——凭证也不该经过浏览器。
            </p>
            <table id="deployTable" class="kv">
              <tbody></tbody>
            </table>
            <p class="lede" style="margin:14px 0 0;">
              gemini-web2api-go · 单二进制 · SQLite · Chrome 146 指纹 · tiktoken cl100k_base 计费
            </p>
          </div>
        </div>
      </div>
    `;
}
