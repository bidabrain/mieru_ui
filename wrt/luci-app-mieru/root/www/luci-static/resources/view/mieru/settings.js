'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require rpc';
'require ui';
'require poll';

var BIN = '/usr/bin/mieru-wrt';

var callServiceList = rpc.declare({
	object: 'service',
	method: 'list',
	params: [ 'name' ],
	expect: { '': {} }
});

function isRunning() {
	return L.resolveDefault(callServiceList('mieru'), {}).then(function(res) {
		var inst = (res.mieru && res.mieru.instances) || {};
		return Object.keys(inst).some(function(k) { return inst[k].running; });
	});
}

function exec(args) {
	return fs.exec(BIN, args).then(function(res) {
		if (res.code !== 0)
			throw new Error((res.stderr || res.stdout || '').trim() || ('exit ' + res.code));
		return (res.stdout || '').trim();
	});
}

function nodeLabel(sid) {
	var name = uci.get('mieru', sid, 'name') || uci.get('mieru', sid, 'server') || sid;
	return name;
}

return view.extend({
	load: function() {
		return Promise.all([
			uci.load('mieru'),
			L.resolveDefault(fs.exec(BIN, [ 'version' ]), {})
		]);
	},

	renderStatus: function(version) {
		var state = E('span', { 'class': 'label' }, '…');
		var detail = E('span', { 'style': 'margin-left:1em' });
		var delayOut = E('span', { 'style': 'margin-left:.5em' });

		var refresh = function() {
			return isRunning().then(function(running) {
				var enabled = uci.get('mieru', 'main', 'enabled') === '1';
				var node = uci.get('mieru', 'main', 'node');
				state.textContent = running ? '运行中' : (enabled ? '未运行（请查看日志）' : '已停用');
				state.style.background = running ? '#2e9d4f' : (enabled ? '#d9534f' : '#888');
				state.style.color = '#fff';
				state.style.padding = '2px 8px';
				state.style.borderRadius = '4px';
				detail.textContent = node ? ('节点：' + nodeLabel(node)) : '尚未选择节点';
			});
		};
		poll.add(refresh, 3);
		refresh();

		var btn = function(label, cls, fn) {
			return E('button', {
				'class': 'cbi-button ' + cls,
				'style': 'margin-right:.5em',
				'click': ui.createHandlerFn(this, fn)
			}, label);
		};

		return E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, '运行状态'),
			E('div', { 'style': 'margin-bottom:.8em' }, [ state, detail ]),
			E('div', { 'style': 'margin-bottom:.8em;color:#888' }, (version.stdout || '').trim() || '未找到 mieru-wrt，请先安装 mieru-wrt 软件包'),
			E('div', {}, [
				btn('测试延迟', 'cbi-button-action', function() {
					delayOut.textContent = '测试中…';
					return exec([ 'delay' ]).then(function(out) {
						var r = JSON.parse(out);
						delayOut.textContent = r.error ? ('失败：' + r.error) : (r.ms + ' ms');
					}).catch(function(e) { delayOut.textContent = '失败：' + e.message; });
				}),
				btn('重启服务', 'cbi-button-apply', function() {
					return fs.exec('/etc/init.d/mieru', [ 'restart' ]).then(refresh);
				}),
				btn('更新规则数据', 'cbi-button-neutral', function() {
					ui.showModal('更新规则数据', [ E('p', { 'class': 'spinning' }, '正在下载 GeoIP / GeoSite …') ]);
					return exec([ 'update-geo' ]).then(function() {
						return fs.exec('/etc/init.d/mieru', [ 'restart' ]);
					}).then(function() {
						ui.hideModal();
						ui.addNotification(null, E('p', '规则数据已更新'), 'info');
					}).catch(function(e) {
						ui.hideModal();
						ui.addNotification(null, E('p', '更新失败：' + e.message), 'danger');
					});
				}),
				delayOut
			])
		]);
	},

	render: function(data) {
		var m, s, o;

		m = new form.Map('mieru', 'Mieru',
			'通过 mieru 协议代理局域网设备的流量（透明代理）。节点在「节点」页添加，可直接粘贴 mierus:// 链接导入。');

		s = m.section(form.NamedSection, 'main', 'mieru');
		s.addremove = false;
		s.tab('basic', '基本设置');
		s.tab('route', '分流规则');
		s.tab('dns', 'DNS');
		s.tab('access', '访问控制');
		s.tab('advanced', '高级');

		/* basic */
		o = s.taboption('basic', form.Flag, 'enabled', '启用');
		o.rmempty = false;

		o = s.taboption('basic', form.ListValue, 'node', '当前节点');
		o.value('', '-- 请选择 --');
		uci.sections('mieru', 'node').forEach(function(sec) {
			o.value(sec['.name'], nodeLabel(sec['.name']));
		});

		o = s.taboption('basic', form.ListValue, 'mode', '分流模式');
		o.value('bypass_cn', '绕过中国大陆（推荐）');
		o.value('gfw', 'GFW 列表（只代理被墙网站）');
		o.value('global', '全局代理');
		o.value('direct', '全部直连');
		o.default = 'bypass_cn';

		/* routing */
		o = s.taboption('route', form.Flag, 'block_ads', '屏蔽广告', '使用 GeoSite category-ads-all 拦截广告域名');
		o = s.taboption('route', form.Flag, 'apple_cn_direct', '苹果 / 微软国内服务直连');
		o.default = '1';
		o.depends('mode', 'bypass_cn');
		o = s.taboption('route', form.Flag, 'sniff', '流量嗅探', '从 TLS / HTTP 中识别域名，让域名规则对直接用 IP 的连接也生效');
		o.default = '1';
		o = s.taboption('route', form.DynamicList, 'rule', '自定义规则',
			'格式：类型,内容,动作，按顺序优先匹配。类型：DOMAIN-SUFFIX、DOMAIN、DOMAIN-KEYWORD、GEOSITE、GEOIP、IP-CIDR、DST-PORT；动作：PROXY、DIRECT、REJECT。<br>' +
			'例：DOMAIN-SUFFIX,openai.com,PROXY　GEOSITE,bilibili,DIRECT　IP-CIDR,1.2.3.0/24,DIRECT');
		o.placeholder = 'DOMAIN-SUFFIX,example.com,PROXY';
		o.validate = function(sid, v) {
			if (!v) return true;
			var p = v.split(',');
			if (p.length !== 3 || !p[1] || /\s/.test(v))
				return '格式应为 类型,内容,动作（不含空格）';
			if ([ 'DOMAIN-SUFFIX', 'DOMAIN', 'DOMAIN-KEYWORD', 'GEOSITE', 'GEOIP', 'IP-CIDR', 'IP-CIDR6', 'DST-PORT' ].indexOf(p[0].toUpperCase()) < 0)
				return '不支持的类型 ' + p[0];
			if ([ 'PROXY', 'DIRECT', 'REJECT' ].indexOf(p[2].toUpperCase()) < 0)
				return '动作必须是 PROXY、DIRECT 或 REJECT';
			return true;
		};

		/* dns */
		o = s.taboption('dns', form.Flag, 'fake_ip', 'Fake-IP',
			'推荐开启：分流更准确、无 DNS 泄漏。关闭则使用真实 IP（redir-host）。');
		o.default = '1';
		o = s.taboption('dns', form.DynamicList, 'direct_dns', '国内 DNS', '用于国内域名和解析节点地址，支持 https:// 、tls:// 和普通 IP');
		o.placeholder = 'https://doh.pub/dns-query';
		o = s.taboption('dns', form.DynamicList, 'remote_dns', '远程 DNS', '经代理查询，用于国外域名');
		o.placeholder = 'https://1.1.1.1/dns-query';
		o = s.taboption('dns', form.Flag, 'dns_hijack', '劫持局域网 DNS',
			'把局域网设备发往任意 DNS 服务器的 53 端口请求都交给路由器处理，防止设备自设 DNS 绕过分流');
		o.default = '1';

		/* access control */
		o = s.taboption('access', form.DynamicList, 'lan_ifname', '局域网接口', '只代理从这些接口进入的流量，一般是 br-lan');
		o.placeholder = 'br-lan';
		o.datatype = 'network';
		o = s.taboption('access', form.ListValue, 'access_mode', '代理哪些设备');
		o.value('all', '所有设备');
		o.value('include', '只代理下面列出的设备');
		o.value('exclude', '不代理下面列出的设备');
		o.default = 'all';
		o = s.taboption('access', form.DynamicList, 'access_ip', '设备 IP', '单个 IPv4 地址或网段，如 192.168.1.20 或 192.168.1.128/25。建议在 DHCP 中给这些设备设置静态地址。');
		o.datatype = 'or(ip4addr,cidr4)';
		o.depends('access_mode', 'include');
		o.depends('access_mode', 'exclude');

		/* advanced */
		o = s.taboption('advanced', form.Value, 'tproxy_port', 'TPROXY 端口');
		o.datatype = 'port';
		o.placeholder = '7893';
		o = s.taboption('advanced', form.Value, 'dns_port', 'DNS 监听端口', '仅监听 127.0.0.1，dnsmasq 会把查询转发到这里');
		o.datatype = 'port';
		o.placeholder = '7874';
		o = s.taboption('advanced', form.Value, 'mixed_port', 'HTTP / SOCKS5 端口', '可选，供局域网设备手动设置代理使用；0 为关闭');
		o.datatype = 'port';
		o.placeholder = '0';
		o = s.taboption('advanced', form.ListValue, 'log_level', '日志级别');
		[ 'debug', 'info', 'warning', 'error', 'silent' ].forEach(function(l) { o.value(l); });
		o.default = 'info';
		o = s.taboption('advanced', form.ListValue, 'geox_source', '规则数据下载源');
		o.value('jsdelivr', 'jsDelivr 镜像（国内可访问）');
		o.value('github', 'GitHub');
		o.default = 'jsdelivr';
		o = s.taboption('advanced', form.Value, 'test_url', '测速地址');
		o.placeholder = 'https://www.gstatic.com/generate_204';

		var status = this.renderStatus(data[1] || {});
		return m.render().then(function(node) {
			var descr = node.querySelector('.cbi-map-descr');
			node.insertBefore(status, descr ? descr.nextSibling : node.firstChild);
			return node;
		});
	}
});
