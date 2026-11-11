// Drives the LuCI pages of luci-app-mieru in headless Chrome (CI end-to-end test).
// usage: node luci.mjs http://<router-ip> <mierus-link-to-import>
import puppeteer from 'puppeteer-core';
import { execSync } from 'node:child_process';

const [base, link] = process.argv.slice(2);
const chrome = execSync('command -v google-chrome || command -v chromium || command -v chromium-browser').toString().trim();
const browser = await puppeteer.launch({ executablePath: chrome, headless: true, args: ['--no-sandbox'] });
const page = await browser.newPage();
await page.setViewport({ width: 1280, height: 900 });

const errors = [];
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
page.on('console', (m) => {
  // Failed requests are reported with their URL by the response handler below.
  if (m.type() === 'error' && !m.text().startsWith('Failed to load resource')) errors.push('console: ' + m.text());
});
page.on('response', (r) => {
  if (r.status() >= 400) errors.push(`HTTP ${r.status()} ${r.request().method()} ${r.url()} ${(r.request().postData() || '').slice(0, 300)}`);
});

const text = () => page.evaluate(() => document.body.innerText);
async function waitText(needle, timeout = 20000) {
  await page.waitForFunction((n) => document.body.innerText.includes(n), { timeout }, needle);
}
function check(cond, msg) {
  if (!cond) throw new Error(msg);
  console.log('ok -', msg);
}

try {
  // Login (the container's root has no password).
  await page.goto(`${base}/cgi-bin/luci/`, { waitUntil: 'networkidle2' });
  await page.evaluate(() => { document.querySelector('input[name=luci_username]').value = 'root'; });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'networkidle2' }),
    page.evaluate(() => document.querySelector('input[name=luci_username]').form.submit()),
  ]);
  // LuCI serves its login form with HTTP 403; only count errors once logged in.
  errors.length = 0;

  // Menu entry under Services.
  await page.goto(`${base}/cgi-bin/luci/admin/services/mieru`, { waitUntil: 'networkidle2' });
  await waitText('运行状态');
  await waitText('运行中');
  let t = await text();
  check(t.includes('mieru-wrt') && t.includes('mihomo'), 'settings: version shown');
  check(t.includes('节点：ci'), 'settings: current node shown');
  for (const tab of ['基本设置', '分流规则', 'DNS', '访问控制', '高级']) check(t.includes(tab), `settings: tab ${tab}`);

  // Latency test button.
  const [delayBtn] = await page.$$('xpath/.//button[contains(., "测试延迟")]');
  await delayBtn.click();
  await waitText(' ms', 30000);
  console.log('ok - settings: delay test shows ms');

  // Nodes page: existing node listed, import another via link.
  await page.goto(`${base}/cgi-bin/luci/admin/services/mieru/nodes`, { waitUntil: 'networkidle2' });
  await waitText('导入节点');
  t = await text();
  check(t.includes('2012/TCP'), 'nodes: existing node in grid');
  await page.type('textarea', link);
  const [importBtn] = await page.$$('xpath/.//button[normalize-space(.)="导入"]');
  await importBtn.click();
  await waitText('已导入 1 个节点', 20000);
  await page.waitForNavigation({ waitUntil: 'networkidle2', timeout: 20000 });
  await waitText('imported');
  console.log('ok - nodes: import adds a node (staged)');

  // Per-node latency button.
  const [rowDelay] = await page.$$('xpath/.//button[normalize-space(.)="测速"]');
  await rowDelay.click();
  await page.waitForFunction(() => /\d+ ms|失败/.test(document.body.innerText), { timeout: 30000 });
  console.log('ok - nodes: row latency button');

  // Log page.
  await page.goto(`${base}/cgi-bin/luci/admin/services/mieru/log`, { waitUntil: 'networkidle2' });
  await waitText('Mieru 日志');
  const log = await page.$eval('textarea', (el) => el.value);
  check(/mieru/.test(log) && !log.includes('读取日志失败'), 'log: shows service log');

  check(errors.length === 0, 'no JavaScript errors' + (errors.length ? ':\n' + errors.join('\n') : ''));
} catch (e) {
  // Printed last-first so the end of the log (what CI shows) has the reason.
  console.error('page text:', (await text().catch(() => '')).slice(0, 800));
  console.error('textarea:', (await page.$eval('textarea', (el) => el.value).catch(() => '')).slice(-400));
  console.error('JS errors:', errors.join('\n') || '(none)');
  console.error('FAILED:', e.message);
  await browser.close();
  process.exit(1);
}
await browser.close();
