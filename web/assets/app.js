'use strict';

// ---------- helpers ----------
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function toast(msg, type = 'ok', ms = 3500) {
  const el = document.createElement('div');
  el.className = 'toast ' + (type === 'ok' ? '' : type);
  el.textContent = msg;
  $('#toasts').appendChild(el);
  setTimeout(() => el.remove(), type === 'err' ? ms * 2 : ms);
}

class ApiError extends Error {
  constructor(msg, obj) { super(msg); this.obj = obj; }
}

async function api(method, path, body) {
  const opt = { method, headers: { 'X-Mieru-UI': '1' }, credentials: 'same-origin' };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, opt);
  } catch (e) {
    throw new ApiError('网络错误: ' + e.message);
  }
  let data;
  try { data = await res.json(); } catch { data = { success: false, msg: 'HTTP ' + res.status }; }
  if (res.status === 401 && path !== 'api/login') {
    showLogin();
    throw new ApiError(data.msg || '未登录');
  }
  if (!data.success) throw new ApiError(data.msg || '请求失败', data.obj);
  return data;
}

// Show the server message; messages containing "失败" (apply warnings) get a warning style.
function report(data) {
  if (!data.msg) return;
  toast(data.msg, data.msg.includes('失败') ? 'warn' : 'ok', data.msg.includes('失败') ? 8000 : 3500);
}

async function withBusy(btn, fn) {
  if (btn) btn.disabled = true;
  try { return await fn(); }
  catch (e) { toast(e.message, 'err'); }
  finally { if (btn) btn.disabled = false; }
}

function fmtBytes(n) {
  if (!n) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(i ? 1 : 0) + ' ' + u[i];
}

function fmtDuration(sec) {
  const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60);
  return (d ? d + ' 天 ' : '') + h + ' 时 ' + m + ' 分';
}

function fmtTime(ms) {
  if (!ms) return '-';
  const d = new Date(ms);
  const p = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

function toLocalInput(ms) {
  if (!ms) return '';
  const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60000);
  return d.toISOString().slice(0, 16);
}

function randomString(n = 16) {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789';
  const buf = new Uint32Array(n);
  crypto.getRandomValues(buf);
  return Array.from(buf, x => chars[x % chars.length]).join('');
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // Fallback for plain-HTTP panels where the Clipboard API is unavailable.
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
  }
  toast('已复制');
}

function download(name, text, type = 'application/json') {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([text], { type }));
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

function store(key, val) {
  try {
    if (val === undefined) return localStorage.getItem(key);
    localStorage.setItem(key, val);
  } catch { return null; }
}

// ---------- theme ----------
function applyTheme(t) {
  if (t) document.documentElement.dataset.theme = t;
}
applyTheme(store('mieru-ui-theme'));
$('#btn-theme').onclick = () => {
  const cur = document.documentElement.dataset.theme ||
    (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  const next = cur === 'dark' ? 'light' : 'dark';
  applyTheme(next);
  store('mieru-ui-theme', next);
};

// ---------- modal ----------
function openModal(title, bodyHTML, buttons = []) {
  $('#modal-title').textContent = title;
  $('#modal-body').innerHTML = bodyHTML;
  const foot = $('#modal-foot');
  foot.innerHTML = '';
  for (const b of buttons) {
    const el = document.createElement('button');
    el.className = 'btn ' + (b.cls || '');
    el.textContent = b.text;
    el.onclick = () => b.onClick(el);
    foot.appendChild(el);
  }
  $('#modal').classList.remove('hidden');
  return $('#modal-body');
}
function closeModal() { $('#modal').classList.add('hidden'); }
$('#modal').addEventListener('click', e => {
  if (e.target.id === 'modal' || e.target.closest('[data-close]')) closeModal();
});
document.addEventListener('keydown', e => { if (e.key === 'Escape') closeModal(); });

function confirmDialog(title, text) {
  return new Promise(resolve => {
    openModal(title, `<p>${esc(text)}</p>`, [
      { text: '取消', onClick: () => { closeModal(); resolve(false); } },
      { text: '确定', cls: 'danger', onClick: () => { closeModal(); resolve(true); } },
    ]);
  });
}

// ---------- auth ----------
function showLogin() {
  $('#app-view').classList.add('hidden');
  $('#login-view').classList.remove('hidden');
  stopTimers();
}

$('#login-form').onsubmit = async e => {
  e.preventDefault();
  const f = new FormData(e.target);
  const btn = $('button', e.target);
  await withBusy(btn, async () => {
    await api('POST', 'api/login', { username: f.get('username'), password: f.get('password') });
    e.target.reset();
    await boot();
  });
};

$('#btn-logout').onclick = async () => {
  try { await api('POST', 'api/logout'); } catch { }
  showLogin();
};

// ---------- navigation ----------
let currentPage = '';
const loaders = {};
let timers = [];

function stopTimers() { timers.forEach(clearInterval); timers = []; }

function show(page) {
  if (!loaders[page]) page = 'dashboard';
  currentPage = page;
  stopTimers();
  $$('[data-section]').forEach(s => s.classList.toggle('hidden', s.dataset.section !== page));
  $$('.sidebar nav a').forEach(a => a.classList.toggle('active', a.dataset.page === page));
  $('#sidebar').classList.remove('open');
  if (location.hash !== '#' + page) history.replaceState(null, '', '#' + page);
  loaders[page]();
}

$$('.sidebar nav a').forEach(a => a.onclick = () => show(a.dataset.page));
$('#menu-toggle').onclick = () => $('#sidebar').classList.toggle('open');
window.addEventListener('hashchange', () => {
  const page = location.hash.slice(1);
  if (page !== currentPage && !$('#app-view').classList.contains('hidden')) show(page);
});

async function boot() {
  let me;
  try { me = (await api('GET', 'api/me')).obj; } catch { showLogin(); return; }
  $('#login-view').classList.add('hidden');
  $('#app-view').classList.remove('hidden');
  $('#me-name').textContent = me.username;
  $('#me-version').textContent = 'v' + me.version;
  $('#default-pass-warn').classList.toggle('hidden', !me.defaultPassword);
  show(location.hash.slice(1) || 'dashboard');
}

// ---------- dashboard ----------
let lastNet = null;

function setBar(id, pct) {
  const el = $(id);
  el.style.width = Math.min(100, pct).toFixed(1) + '%';
  el.classList.toggle('hot', pct > 85);
}

async function refreshStatus() {
  let s;
  try { s = (await api('GET', 'api/status')).obj; } catch { return; }
  $('#st-cpu').textContent = s.cpuUsage.toFixed(1) + '%  · ' + s.cpuCores + ' 核';
  setBar('#bar-cpu', s.cpuUsage);
  $('#st-mem').textContent = s.memTotal ? `${fmtBytes(s.memUsed)} / ${fmtBytes(s.memTotal)}` : '-';
  setBar('#bar-mem', s.memTotal ? s.memUsed / s.memTotal * 100 : 0);
  $('#st-disk').textContent = s.diskTotal ? `${fmtBytes(s.diskUsed)} / ${fmtBytes(s.diskTotal)}` : '-';
  setBar('#bar-disk', s.diskTotal ? s.diskUsed / s.diskTotal * 100 : 0);
  $('#st-uptime').textContent = s.uptime ? fmtDuration(s.uptime) : '-';
  $('#st-load').textContent = s.load ? '负载 ' + s.load : '';
  $('#st-net').textContent = `总计 ↑ ${fmtBytes(s.netSent)} / ↓ ${fmtBytes(s.netRecv)}`;
  if (lastNet && s.time > lastNet.time) {
    const dt = (s.time - lastNet.time) / 1000;
    const up = Math.max(0, (s.netSent - lastNet.netSent) / dt), down = Math.max(0, (s.netRecv - lastNet.netRecv) / dt);
    $('#st-speed').textContent = `${fmtBytes(up)}/s · ${fmtBytes(down)}/s`;
  }
  lastNet = s;
  $('#st-conns').textContent = `${s.tcpConns} / ${s.udpConns}`;
  $('#st-os').textContent = `${s.os}/${s.arch}`;
}

async function refreshMita() {
  let m;
  try { m = (await api('GET', 'api/mita')).obj; } catch { return; }
  const badge = $('#mita-badge');
  if (!m.installed) {
    badge.textContent = '未安装'; badge.className = 'badge bad';
  } else if (m.running) {
    badge.textContent = '运行中'; badge.className = 'badge ok';
  } else {
    badge.textContent = '未运行'; badge.className = 'badge warn';
  }
  $('#mita-version').textContent = m.installed ? (m.version || '-') : '未找到 mita，请先安装 mieru 服务端（见 README）';
  $('#mita-status').textContent = m.statusText || '-';
  $('#mita-users').textContent = `${m.activeUsers} 个生效 / 共 ${m.totalUsers} 个`;
  $('#mita-apply').innerHTML = m.lastApplyAt
    ? esc(fmtTime(m.lastApplyAt)) + (m.lastApplyError ? ` <span class="badge bad">失败</span> <span class="mono">${esc(m.lastApplyError)}</span>` : ' <span class="badge ok">成功</span>')
    : '-';
}

loaders.dashboard = () => {
  lastNet = null;
  refreshStatus();
  refreshMita();
  timers.push(setInterval(refreshStatus, 3000));
  timers.push(setInterval(refreshMita, 10000));
};

$$('[data-mita]').forEach(btn => btn.onclick = async () => {
  const action = btn.dataset.mita;
  if (action === 'stop' && !await confirmDialog('停止 mita', '停止后所有用户将无法连接，确定吗？')) return;
  const out = $('#mita-steps');
  await withBusy(btn, async () => {
    try {
      const r = await api('POST', 'api/mita/' + action);
      toast(r.msg);
      out.textContent = (r.obj || []).join('\n');
    } catch (e) {
      out.textContent = ((e.obj || []).join('\n') + '\n' + e.message).trim();
      throw e;
    } finally {
      out.classList.remove('hidden');
      refreshMita();
    }
  });
});

$('#btn-mita-users').onclick = e => withBusy(e.target, async () => {
  $('#mita-users-out').textContent = (await api('GET', 'api/mita/users')).obj || '(空)';
});
$('#btn-mita-metrics').onclick = e => withBusy(e.target, async () => {
  $('#mita-metrics-out').textContent = (await api('GET', 'api/mita/metrics')).obj || '(空)';
});

// ---------- users ----------
let users = [];

loaders.users = async () => {
  try { users = (await api('GET', 'api/users')).obj || []; } catch (e) { toast(e.message, 'err'); return; }
  renderUsers();
};

function quotaText(q) {
  if (!q || !q.length) return '<span class="muted">不限</span>';
  return q.map(x => `<span class="tag">${x.days} 天 ${fmtBytes(x.megabytes * 1024 * 1024)}</span>`).join('');
}

function expiryText(ms) {
  if (!ms) return '<span class="muted">永久</span>';
  const left = ms - Date.now();
  if (left <= 0) return `<span class="badge bad">已过期</span> <span class="muted small">${esc(fmtTime(ms))}</span>`;
  const days = Math.ceil(left / 86400000);
  return `${esc(fmtTime(ms))} <span class="badge ${days < 3 ? 'warn' : ''}">剩 ${days} 天</span>`;
}

function renderUsers() {
  const kw = $('#user-search').value.trim().toLowerCase();
  const list = users.filter(u => !kw || u.name.toLowerCase().includes(kw) || (u.remark || '').toLowerCase().includes(kw));
  $('#user-empty').classList.toggle('hidden', users.length > 0);
  $('#user-rows').innerHTML = list.map(u => `
    <tr data-id="${u.id}">
      <td><label class="switch"><input type="checkbox" data-act="toggle" ${u.enabled ? 'checked' : ''}><span></span></label></td>
      <td class="mono">${esc(u.name)}</td>
      <td>${esc(u.remark) || '<span class="muted">-</span>'}</td>
      <td>${quotaText(u.quotas)}</td>
      <td>${expiryText(u.expireAt)}</td>
      <td>${u.allowPrivateIP ? '<span class="tag">内网</span>' : ''}${u.allowLoopbackIP ? '<span class="tag">回环</span>' : ''}${!u.allowPrivateIP && !u.allowLoopbackIP ? '<span class="muted">-</span>' : ''}</td>
      <td><div class="btn-row">
        <button class="btn small" data-act="share">分享</button>
        <button class="btn small" data-act="edit">编辑</button>
        <button class="btn small danger" data-act="del">删除</button>
      </div></td>
    </tr>`).join('');
}

$('#user-search').oninput = renderUsers;

$('#user-rows').addEventListener('click', async e => {
  const el = e.target.closest('[data-act]');
  if (!el) return;
  const id = Number(el.closest('tr').dataset.id);
  const u = users.find(x => x.id === id);
  if (!u) return;
  const act = el.dataset.act;
  if (act === 'edit') userModal(u);
  else if (act === 'share') shareModal(u);
  else if (act === 'del') {
    if (!await confirmDialog('删除用户', `确定删除用户 ${u.name} 吗？该用户将立即无法连接。`)) return;
    await withBusy(el, async () => { report(await api('DELETE', `api/users/${id}`)); await loaders.users(); });
  }
});

$('#user-rows').addEventListener('change', async e => {
  const el = e.target.closest('[data-act="toggle"]');
  if (!el) return;
  const id = Number(el.closest('tr').dataset.id);
  el.disabled = true;
  try {
    report(await api('POST', `api/users/${id}/enable`, { enabled: el.checked }));
    await loaders.users();
  } catch (err) {
    toast(err.message, 'err');
    el.checked = !el.checked;
  } finally { el.disabled = false; }
});

function quotaRowHTML(q = { days: 30, megabytes: 102400 }) {
  return `<div class="quota-row">
    <input type="number" min="1" class="q-days" value="${q.days}" placeholder="周期（天）">
    <input type="number" min="0.01" step="0.01" class="q-gb" value="${+(q.megabytes / 1024).toFixed(2)}" placeholder="流量（GB）">
    <button class="btn small danger" type="button" data-rm>删除</button>
  </div>`;
}

function readQuotas(root) {
  return $$('.quota-row', root).map(r => ({
    days: parseInt($('.q-days', r).value, 10),
    megabytes: Math.round(parseFloat($('.q-gb', r).value) * 1024),
  })).filter(q => q.days > 0 && q.megabytes > 0);
}

function userModal(u) {
  const isNew = !u;
  u = u || { name: '', password: randomString(16), enabled: true, remark: '', quotas: [], expireAt: 0 };
  const body = openModal(isNew ? '添加用户' : '编辑用户 ' + u.name, `
    <div class="row">
      <label>用户名<input id="f-name" value="${esc(u.name)}" placeholder="留空自动生成"></label>
      <label>备注<input id="f-remark" value="${esc(u.remark)}"></label>
    </div>
    <label>密码
      <div class="copy-box"><input id="f-pass" value="${esc(u.password)}"><button class="btn" type="button" id="f-gen">随机</button></div>
    </label>
    <div class="row">
      <label>到期时间 <small>留空为永久，到期自动禁用</small><input type="datetime-local" id="f-exp" value="${toLocalInput(u.expireAt)}"></label>
      <label>快捷设置
        <select id="f-exp-quick"><option value="">选择…</option><option value="0">永久</option><option value="7">7 天后</option><option value="30">30 天后</option><option value="90">90 天后</option><option value="365">1 年后</option></select>
      </label>
    </div>
    <div>
      <div class="card-head" style="margin-bottom:8px"><strong>流量配额</strong><button class="btn small" type="button" id="f-addq">添加配额</button></div>
      <p class="muted small" style="margin:0 0 8px">在滑动周期内超出流量即被 mita 限制。可设置多条，例如「1 天 10GB」+「30 天 200GB」。不添加即不限流量。</p>
      <div id="f-quotas" style="display:flex;flex-direction:column;gap:8px">${(u.quotas || []).map(quotaRowHTML).join('')}</div>
    </div>
    <label class="check"><input type="checkbox" id="f-enabled" ${u.enabled ? 'checked' : ''}> 启用</label>
    <label class="check"><input type="checkbox" id="f-private" ${u.allowPrivateIP ? 'checked' : ''}> 允许访问服务器内网 IP</label>
    <label class="check"><input type="checkbox" id="f-loop" ${u.allowLoopbackIP ? 'checked' : ''}> 允许访问服务器回环地址 (127.0.0.1)</label>
  `, [
    { text: '取消', onClick: closeModal },
    {
      text: '保存', cls: 'primary', onClick: btn => withBusy(btn, async () => {
        const exp = $('#f-exp').value;
        const data = {
          name: $('#f-name').value.trim(),
          password: $('#f-pass').value,
          remark: $('#f-remark').value.trim(),
          enabled: $('#f-enabled').checked,
          allowPrivateIP: $('#f-private').checked,
          allowLoopbackIP: $('#f-loop').checked,
          quotas: readQuotas(body),
          expireAt: exp ? new Date(exp).getTime() : 0,
        };
        const r = isNew ? await api('POST', 'api/users', data) : await api('PUT', `api/users/${u.id}`, data);
        report(r);
        closeModal();
        await loaders.users();
      })
    },
  ]);
  $('#f-gen').onclick = () => { $('#f-pass').value = randomString(16); };
  $('#f-addq').onclick = () => $('#f-quotas').insertAdjacentHTML('beforeend', quotaRowHTML());
  $('#f-quotas').onclick = e => { if (e.target.matches('[data-rm]')) e.target.closest('.quota-row').remove(); };
  $('#f-exp-quick').onchange = e => {
    const v = e.target.value;
    if (v === '') return;
    $('#f-exp').value = v === '0' ? '' : toLocalInput(Date.now() + Number(v) * 86400000);
    e.target.value = '';
  };
}

$('#btn-add-user').onclick = () => userModal(null);

$('#btn-batch').onclick = () => {
  const body = openModal('批量添加用户', `
    <div class="row">
      <label>数量<input type="number" id="b-count" value="5" min="1" max="500"></label>
      <label>用户名前缀<input id="b-prefix" value="user"></label>
    </div>
    <div class="row">
      <label>有效天数 <small>0 为永久</small><input type="number" id="b-days" value="0" min="0"></label>
      <label>备注<input id="b-remark"></label>
    </div>
    <div>
      <div class="card-head" style="margin-bottom:8px"><strong>流量配额</strong><button class="btn small" type="button" id="b-addq">添加配额</button></div>
      <div id="b-quotas" style="display:flex;flex-direction:column;gap:8px"></div>
    </div>
  `, [
    { text: '取消', onClick: closeModal },
    {
      text: '添加', cls: 'primary', onClick: btn => withBusy(btn, async () => {
        const days = parseInt($('#b-days').value, 10) || 0;
        report(await api('POST', 'api/users/batch', {
          count: parseInt($('#b-count').value, 10),
          prefix: $('#b-prefix').value.trim(),
          remark: $('#b-remark').value.trim(),
          quotas: readQuotas(body),
          expireAt: days > 0 ? Date.now() + days * 86400000 : 0,
        }));
        closeModal();
        await loaders.users();
      })
    },
  ]);
  $('#b-addq').onclick = () => $('#b-quotas').insertAdjacentHTML('beforeend', quotaRowHTML());
  $('#b-quotas').onclick = e => { if (e.target.matches('[data-rm]')) e.target.closest('.quota-row').remove(); };
};

async function shareModal(u, host = '') {
  let s;
  try {
    s = (await api('GET', `api/users/${u.id}/share` + (host ? '?host=' + encodeURIComponent(host) : ''))).obj;
  } catch (e) { toast(e.message, 'err'); return; }
  openModal('分享 ' + u.name, `
    ${u.enabled ? '' : '<div class="alert warn">该用户当前已禁用，配置无法连接。</div>'}
    <label>服务器地址 <small>可在「面板设置 → 分享地址」中设置默认值</small>
      <div class="copy-box"><input id="s-host" value="${esc(s.host)}"><button class="btn" id="s-host-apply" type="button">更新</button></div>
    </label>
    <div class="tabs">
      <button class="active" data-tab="link">分享链接</button>
      <button data-tab="client">mieru 客户端配置</button>
      <button data-tab="mihomo">mihomo / Clash.Meta</button>
    </div>
    <div data-pane="link">
      <div class="qr"><img alt="QR" src="api/qrcode?text=${encodeURIComponent(s.link)}"></div>
      <div class="copy-box" style="margin-top:12px"><input readonly value="${esc(s.link)}"><button class="btn primary" type="button" data-copy="link">复制</button></div>
      <p class="muted small">mierus:// 简单分享链接，可导入 mieru 客户端（<span class="mono">mieru import config &lt;链接&gt;</span>）以及支持该格式的 GUI 客户端。</p>
    </div>
    <div data-pane="client" class="hidden">
      <textarea class="mono" rows="14" readonly>${esc(s.clientJSON)}</textarea>
      <div class="btn-row" style="margin-top:8px">
        <button class="btn primary" type="button" data-copy="clientJSON">复制</button>
        <button class="btn" type="button" id="s-dl">下载 client.json</button>
      </div>
      <p class="muted small">使用 <span class="mono">mieru apply config client.json</span> 导入，然后 <span class="mono">mieru start</span>。本地 SOCKS5 端口 1080，HTTP 端口 8080。</p>
    </div>
    <div data-pane="mihomo" class="hidden">
      <textarea class="mono" rows="14" readonly>${esc(s.mihomo)}</textarea>
      <div class="btn-row" style="margin-top:8px"><button class="btn primary" type="button" data-copy="mihomo">复制</button></div>
      <p class="muted small">粘贴到 mihomo 配置的 proxies 段（需要 mihomo 1.18.5+ 才支持 mieru）。</p>
    </div>
  `);
  const body = $('#modal-body');
  $$('.tabs button', body).forEach(b => b.onclick = () => {
    $$('.tabs button', body).forEach(x => x.classList.toggle('active', x === b));
    $$('[data-pane]', body).forEach(p => p.classList.toggle('hidden', p.dataset.pane !== b.dataset.tab));
  });
  $$('[data-copy]', body).forEach(b => b.onclick = () => copyText(s[b.dataset.copy]));
  $('#s-dl').onclick = () => download(`mieru-${u.name}.json`, s.clientJSON);
  $('#s-host-apply').onclick = () => shareModal(u, $('#s-host').value.trim());
}

// ---------- server config (ports & advanced) ----------
let serverConf = null;

async function loadServer() {
  serverConf = (await api('GET', 'api/server')).obj;
  serverConf.portBindings = serverConf.portBindings || [];
}

async function saveServer(btn) {
  await withBusy(btn, async () => {
    const r = await api('PUT', 'api/server', serverConf);
    report(r);
    serverConf = r.obj || serverConf;
  });
}

loaders.ports = async () => {
  try { await loadServer(); } catch (e) { toast(e.message, 'err'); return; }
  renderPorts();
};

function renderPorts() {
  $('#port-rows').innerHTML = serverConf.portBindings.map((b, i) => {
    const isRange = !!b.portRange;
    return `<tr data-i="${i}">
      <td><select data-f="kind"><option value="single" ${isRange ? '' : 'selected'}>单端口</option><option value="range" ${isRange ? 'selected' : ''}>端口范围</option></select></td>
      <td><input data-f="value" value="${esc(isRange ? b.portRange : (b.port || ''))}" placeholder="${isRange ? '例如 2000-2010' : '例如 2012'}"></td>
      <td><select data-f="protocol"><option ${b.protocol === 'TCP' ? 'selected' : ''}>TCP</option><option ${b.protocol === 'UDP' ? 'selected' : ''}>UDP</option></select></td>
      <td><div class="btn-row"><button class="btn small danger" data-rm>删除</button></div></td>
    </tr>`;
  }).join('') || '<tr><td colspan="4" class="empty">没有端口，请添加。</td></tr>';
}

function readPorts() {
  return $$('#port-rows tr[data-i]').map(tr => {
    const kind = $('[data-f=kind]', tr).value;
    const value = $('[data-f=value]', tr).value.trim();
    const protocol = $('[data-f=protocol]', tr).value;
    return kind === 'range' ? { portRange: value, protocol } : { port: parseInt(value, 10) || 0, protocol };
  });
}

$('#port-rows').addEventListener('click', e => {
  if (!e.target.matches('[data-rm]')) return;
  serverConf.portBindings = readPorts();
  serverConf.portBindings.splice(Number(e.target.closest('tr').dataset.i), 1);
  renderPorts();
});
$('#port-rows').addEventListener('change', e => {
  if (e.target.matches('[data-f=kind]')) { serverConf.portBindings = readPorts(); renderPorts(); }
});
$('#btn-add-port').onclick = () => {
  serverConf.portBindings = readPorts();
  const used = serverConf.portBindings.map(b => b.port).filter(Boolean);
  let p = 2012;
  while (used.includes(p)) p++;
  serverConf.portBindings.push({ port: p, protocol: 'TCP' });
  renderPorts();
};
$('#btn-save-ports').onclick = async e => {
  serverConf.portBindings = readPorts();
  await saveServer(e.target);
  renderPorts();
};

const TP_TEMPLATE = {
  seed: Math.floor(Math.random() * 2 ** 31),
  tcpFragment: { enable: true, maxSleepMs: 100 },
  nonce: { type: 'NONCE_TYPE_PRINTABLE', applyToAllUDPPacket: true, minLen: 6, maxLen: 8 },
};
const EGRESS_TEMPLATE = {
  proxies: [{ name: 'warp', protocol: 'SOCKS5_PROXY_PROTOCOL', host: '127.0.0.1', port: 40000 }],
  rules: [{ ipRanges: ['*'], domainNames: ['*'], action: 'PROXY', proxyNames: ['warp'] }],
};
const pretty = v => (v && Object.keys(v).length ? JSON.stringify(v, null, 2) : '');

loaders.advanced = async () => {
  try { await loadServer(); } catch (e) { toast(e.message, 'err'); return; }
  $('#adv-mtu').value = serverConf.mtu;
  $('#adv-log').value = serverConf.loggingLevel;
  $('#adv-dual').value = serverConf.dualStack || '';
  $('#adv-mux').value = serverConf.multiplexing || '';
  $('#adv-hs').value = serverConf.handshakeMode || '';
  $('#adv-tp').value = pretty(serverConf.trafficPattern);
  $('#adv-egress').value = pretty(serverConf.egress);
};

function parseJSONField(id, name) {
  const t = $(id).value.trim();
  if (!t) return null;
  try { return JSON.parse(t); } catch (e) { throw new Error(`${name} 不是合法 JSON: ${e.message}`); }
}

$('#btn-save-adv').onclick = async e => {
  try {
    serverConf.mtu = parseInt($('#adv-mtu').value, 10);
    serverConf.loggingLevel = $('#adv-log').value;
    serverConf.dualStack = $('#adv-dual').value;
    serverConf.multiplexing = $('#adv-mux').value;
    serverConf.handshakeMode = $('#adv-hs').value;
    serverConf.trafficPattern = parseJSONField('#adv-tp', 'trafficPattern');
    serverConf.egress = parseJSONField('#adv-egress', 'egress');
  } catch (err) { toast(err.message, 'err'); return; }
  await saveServer(e.target);
};
$('#btn-tp-template').onclick = () => { $('#adv-tp').value = pretty(TP_TEMPLATE); };
$('#btn-egress-template').onclick = () => { $('#adv-egress').value = pretty(EGRESS_TEMPLATE); };
$('#btn-preview').onclick = e => withBusy(e.target, async () => {
  const p = (await api('GET', 'api/server/preview')).obj;
  openModal('mita 配置预览', `
    <div><strong>面板生成的配置</strong><pre class="mono out">${esc(p.generated)}</pre></div>
    <div><strong>mita 当前生效的配置</strong> <span class="muted small">(mita describe config)</span><pre class="mono out">${esc(p.effective || '(mita 未安装或无输出)')}</pre></div>
  `);
});

// ---------- firewall ----------
let fw = null;

const FW_NAMES = { ufw: 'ufw', firewalld: 'firewalld', iptables: 'iptables（仅查看）', none: '未检测到' };
const actionText = a => ({ allow: '允许', deny: '拒绝', reject: '拒绝(reject)', limit: '限速允许' }[a] || a);
const protoText = p => (p === 'both' ? 'TCP+UDP' : (p || '-').toUpperCase());

loaders.firewall = async () => {
  try { fw = (await api('GET', 'api/firewall')).obj; } catch (e) { toast(e.message, 'err'); return; }
  const badge = $('#fw-badge');
  if (fw.backend === 'none') { badge.textContent = '不可用'; badge.className = 'badge bad'; }
  else if (fw.active) { badge.textContent = '已启用'; badge.className = 'badge ok'; }
  else { badge.textContent = '未启用'; badge.className = 'badge warn'; }

  $('#fw-backend').textContent = FW_NAMES[fw.backend] || fw.backend;
  $('#fw-defaults').textContent = fw.defaults || '-';
  $('#fw-ssh').textContent = (fw.sshPorts || []).join(', ') || '-';
  const msgs = [];
  if (fw.message) msgs.push(fw.message);
  if (fw.editable && !fw.active) msgs.push('防火墙当前未启用：所有端口均对外开放，下面的规则启用后才会生效。');
  $('#fw-msg').textContent = msgs.join(' ');
  $('#fw-msg').classList.toggle('hidden', !msgs.length);

  $('#btn-fw-enable').classList.toggle('hidden', !fw.editable || fw.active);
  $('#btn-fw-disable').classList.toggle('hidden', !fw.editable || !fw.active);
  $('#btn-fw-install').classList.toggle('hidden', !fw.canInstallUFW);
  $('#fw-checks-card').classList.toggle('hidden', !fw.editable);
  $('#fw-rules-card').classList.toggle('hidden', !fw.editable);

  $('#fw-checks').innerHTML = (fw.checks || []).map((c, i) => `
    <tr>
      <td>${esc(c.label)}</td><td class="mono">${esc(c.port)}</td><td>${protoText(c.proto)}</td>
      <td>${c.allowed ? '<span class="badge ok">已放行</span>' : '<span class="badge bad">未放行</span>'}</td>
      <td><div class="btn-row">${c.allowed ? '' : `<button class="btn small primary" data-check="${i}">放行</button>`}</div></td>
    </tr>`).join('');

  $('#fw-rules').innerHTML = (fw.rules || []).filter((r, i, arr) => arr.findIndex(x => x.id === r.id) === i).map(r => `
    <tr>
      <td class="mono">${esc(r.text)}</td>
      <td>${esc(actionText(r.action))}</td>
      <td class="mono">${esc(r.port || '-')}</td>
      <td>${protoText(r.proto)}</td>
      <td><div class="btn-row"><button class="btn small danger" data-fw-del="${esc(r.id)}">删除</button></div></td>
    </tr>`).join('') || '<tr><td colspan="5" class="empty">没有规则</td></tr>';
  $('#fw-raw').textContent = fw.raw || '(空)';
};

async function fwAction(btn, fn) {
  await withBusy(btn, async () => {
    report(await fn());
    await loaders.firewall();
  });
}

$('#btn-fw-refresh').onclick = e => withBusy(e.target, loaders.firewall);
$('#btn-fw-add').onclick = e => fwAction(e.target, () =>
  api('POST', 'api/firewall/allow', { port: $('#fw-port').value.trim(), proto: $('#fw-proto').value }));
$('#btn-fw-open-mieru').onclick = e => fwAction(e.target, () => api('POST', 'api/firewall/open-mieru'));
$('#fw-checks').addEventListener('click', e => {
  const b = e.target.closest('[data-check]');
  if (!b) return;
  const c = fw.checks[Number(b.dataset.check)];
  fwAction(b, () => api('POST', 'api/firewall/allow', { port: c.port, proto: c.proto }));
});
$('#fw-rules').addEventListener('click', async e => {
  const b = e.target.closest('[data-fw-del]');
  if (!b) return;
  const id = b.dataset.fwDel;
  if (!await confirmDialog('删除规则', `确定删除防火墙规则「${id}」吗？`)) return;
  await withBusy(b, async () => {
    try {
      report(await api('POST', 'api/firewall/delete', { id }));
    } catch (err) {
      if (err.obj !== 'need_force') throw err;
      if (!await confirmDialog('危险操作', err.message + '。仍然要删除吗？')) return;
      report(await api('POST', 'api/firewall/delete', { id, force: true }));
    }
    await loaders.firewall();
  });
});
$('#btn-fw-enable').onclick = async e => {
  if (!await confirmDialog('启用防火墙', `启用后只有已放行的端口可以访问。面板会先自动放行 SSH（${(fw.sshPorts || []).join(', ')}）和面板端口。确定启用吗？`)) return;
  fwAction(e.target, () => api('POST', 'api/firewall/enable'));
};
$('#btn-fw-disable').onclick = async e => {
  if (!await confirmDialog('关闭防火墙', '关闭后服务器所有端口都将对外开放，确定吗？')) return;
  fwAction(e.target, () => api('POST', 'api/firewall/disable'));
};
$('#btn-fw-install').onclick = e => fwAction(e.target, () => api('POST', 'api/firewall/install-ufw'));

// ---------- panel settings ----------
loaders.panel = async () => {
  let s;
  try { s = (await api('GET', 'api/settings')).obj; } catch (e) { toast(e.message, 'err'); return; }
  $('#set-listen').value = s.listenIP;
  $('#set-port').value = s.port;
  $('#set-base').value = s.basePath;
  $('#set-session').value = s.sessionHours;
  $('#set-host').value = s.publicHost;
  $('#set-mita').value = s.mitaBin;
  $('#set-cert').value = s.certFile;
  $('#set-key').value = s.keyFile;
  $('#cred-user').value = $('#me-name').textContent;
};

$('#btn-save-settings').onclick = e => withBusy(e.target, async () => {
  report(await api('PUT', 'api/settings', {
    listenIP: $('#set-listen').value.trim(),
    port: parseInt($('#set-port').value, 10),
    basePath: $('#set-base').value.trim(),
    sessionHours: parseInt($('#set-session').value, 10),
    publicHost: $('#set-host').value.trim(),
    mitaBin: $('#set-mita').value.trim(),
    certFile: $('#set-cert').value.trim(),
    keyFile: $('#set-key').value.trim(),
  }));
});

$('#btn-restart-panel').onclick = async e => {
  if (!await confirmDialog('重启面板', '面板将重启，新的端口/路径/证书设置会生效。若修改了端口或路径，请使用新地址访问。')) return;
  await withBusy(e.target, async () => {
    report(await api('POST', 'api/panel/restart'));
    setTimeout(() => location.reload(), 4000);
  });
};

$('#btn-save-cred').onclick = e => withBusy(e.target, async () => {
  report(await api('PUT', 'api/settings/credentials', {
    oldPassword: $('#cred-old').value,
    username: $('#cred-user').value.trim(),
    password: $('#cred-pass').value,
  }));
  $('#cred-old').value = $('#cred-pass').value = '';
  setTimeout(showLogin, 800);
});

$('#restore-file').onchange = async e => {
  const file = e.target.files[0];
  e.target.value = '';
  if (!file) return;
  if (!await confirmDialog('恢复备份', `将用 ${file.name} 覆盖当前所有用户和服务端配置，确定吗？`)) return;
  try {
    const doc = JSON.parse(await file.text());
    report(await api('POST', 'api/restore', doc));
  } catch (err) { toast(err.message, 'err'); }
};

// ---------- logs ----------
async function loadLogs(btn) {
  await withBusy(btn, async () => {
    const out = $('#log-out');
    out.textContent = (await api('GET', `api/logs?unit=${$('#log-unit').value}&n=${$('#log-n').value}`)).obj || '(空)';
    out.scrollTop = out.scrollHeight;
  });
}
loaders.logs = () => loadLogs();
$('#btn-logs').onclick = e => loadLogs(e.target);
$('#log-unit').onchange = () => loadLogs();

boot();
