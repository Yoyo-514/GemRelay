async function loadSettingsPage() { const k = await api("api/apikey"); return renderSettingsPage(k); }
function renderSettingsPage(k) {
    return `
       <div class="grid-main settings-layout">
         <div class="panel">
          <h2>运行时配置</h2>
          <p class="lede">
            保存后更新运行时配置，优先级高于 <code>config.json</code> 和命令行参数。
            上游超时会缓存在网络客户端中，修改后请重启服务以更新已有连接客户端。
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
                : '给 <code>/v1/*</code> 和 <code>/mcp</code> 接口鉴权，可随时轮换。'}
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
              监听、数据库和管理员令牌在部署时设置，可通过启动参数或 Compose 环境变量管理。
              Google Cookie 在「Cookie 池」导入；API Key 在上方管理。
            </p>
            <table id="deployTable" class="kv">
              <tbody></tbody>
            </table>
            <p class="lede" style="margin:14px 0 0;">
              GemRelay · 单二进制 · token 数为本地估算
            </p>
          </div>
        </div>
      </div>
    `;
}
