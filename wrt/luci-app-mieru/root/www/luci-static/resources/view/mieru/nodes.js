'use strict';
'require view';
'require form';
'require uci';
'require fs';
'require ui';

var BIN = '/usr/bin/mieru-wrt';

function exec(args) {
	return fs.exec(BIN, args).then(function(res) {
		if (res.code !== 0)
			throw new Error((res.stderr || res.stdout || '').trim() || ('exit ' + res.code));
		return (res.stdout || '').trim();
	});
}

function importLinks(text) {
	text = (text || '').trim();
	if (!text)
		return Promise.reject(new Error('请先粘贴 mierus:// 链接'));
	return exec([ 'parse-link', text ]).then(function(out) {
		var nodes = JSON.parse(out);
		var first = null;
		nodes.forEach(function(n) {
			var sid = uci.add('mieru', 'node');
			first = first || sid;
			uci.set('mieru', sid, 'name', n.name || n.server);
			uci.set('mieru', sid, 'server', n.server);
			uci.set('mieru', sid, 'username', n.username);
			uci.set('mieru', sid, 'password', n.password);
			uci.set('mieru', sid, 'port', n.port || []);
			if (n.mtu) uci.set('mieru', sid, 'mtu', String(n.mtu));
			if (n.multiplexing) uci.set('mieru', sid, 'multiplexing', n.multiplexing);
			if (n.handshake_mode) uci.set('mieru', sid, 'handshake_mode', n.handshake_mode);
		});
		if (first && !uci.get('mieru', 'main', 'node'))
			uci.set('mieru', 'main', 'node', first);
		return uci.save().then(function() { return nodes.length; });
	});
}

return view.extend({
	load: function() {
		return uci.load('mieru');
	},

	renderImport: function() {
		var ta = E('textarea', {
			'class': 'cbi-input-textarea',
			'style': 'width:100%;font-family:monospace',
			'rows': 3,
			'placeholder': 'mierus://用户名:密码@服务器?port=2012&protocol=TCP&profile=名称（每行一个，可一次粘贴多个）'
		});
		return E('div', { 'class': 'cbi-section' }, [
			E('h3', {}, '导入节点'),
			E('p', {}, '粘贴面板「分享链接」中的 mierus:// 链接，或 mieru 客户端 JSON 配置。'),
			ta,
			E('div', { 'style': 'margin-top:.5em' }, E('button', {
				'class': 'cbi-button cbi-button-add',
				'click': ui.createHandlerFn(this, function() {
					return importLinks(ta.value).then(function(n) {
						ui.addNotification(null, E('p', '已导入 ' + n + ' 个节点，点击右上角「保存并应用」生效'), 'info');
						window.setTimeout(function() { window.location.reload(); }, 800);
					}).catch(function(e) {
						ui.addNotification(null, E('p', '导入失败：' + e.message), 'danger');
					});
				})
			}, '导入'))
		]);
	},

	render: function() {
		var m, s, o;

		m = new form.Map('mieru', '节点', '在「基本设置」中选择要使用的节点。修改后点击「保存并应用」。');

		s = m.section(form.GridSection, 'node');
		s.addremove = true;
		s.anonymous = true;
		s.sortable = true;
		s.nodescriptions = true;
		s.addbtntitle = '手动添加节点';
		s.modaltitle = function(sid) { return '节点：' + (uci.get('mieru', sid, 'name') || sid); };

		s.renderRowActions = function(sid) {
			var td = form.GridSection.prototype.renderRowActions.apply(this, [ sid ]);
			var out = E('span', { 'style': 'margin-right:.5em;display:inline-block;max-width:22em;white-space:normal;font-size:90%' });
			var btn = E('button', {
				'class': 'cbi-button cbi-button-action',
				'title': '测试到这个节点的延迟（未保存的修改也会生效）',
				'click': ui.createHandlerFn(this, function() {
					out.textContent = '…';
					out.style.color = '';
					// Test the node as shown here, including changes not yet saved & applied.
					var node = uci.get('mieru', sid) || {};
					var js = JSON.stringify({
						'.name': sid, name: node.name, server: node.server, username: node.username,
						password: node.password, port: node.port, mtu: node.mtu,
						multiplexing: node.multiplexing, handshake_mode: node.handshake_mode
					});
					var fail = function(msg) { out.textContent = '失败：' + msg; out.style.color = '#d9534f'; };
					return exec([ 'delay', '--json', js ]).then(function(res) {
						var r = JSON.parse(res);
						if (r.error) fail(r.error);
						else out.textContent = r.ms + ' ms';
					}).catch(function(e) { fail(e.message); });
				})
			}, '测速');
			var box = td.lastElementChild || td;
			box.insertBefore(btn, box.firstChild);
			box.insertBefore(out, box.firstChild);
			return td;
		};

		o = s.option(form.Value, 'name', '名称');
		o.rmempty = true;

		o = s.option(form.Value, 'server', '服务器地址');
		o.datatype = 'host';
		o.rmempty = false;

		o = s.option(form.DynamicList, 'port', '端口', '格式：端口/协议，如 2012/TCP，或端口范围 3000-3010/UDP');
		o.rmempty = false;
		o.validate = function(sid, v) {
			if (!v) return true;
			var m = /^(\d{1,5})(?:-(\d{1,5}))?(?:\/(tcp|udp))?$/i.exec(v);
			if (!m) return '格式应为 2012/TCP 或 3000-3010/UDP';
			var lo = +m[1], hi = m[2] ? +m[2] : lo;
			if (lo < 1 || hi > 65535 || lo > hi) return '端口范围无效';
			return true;
		};
		o.textvalue = function(sid) { return (uci.get('mieru', sid, 'port') || []).join(', '); };

		o = s.option(form.Value, 'username', '用户名');
		o.modalonly = true;
		o.rmempty = false;

		o = s.option(form.Value, 'password', '密码');
		o.modalonly = true;
		o.password = true;
		o.rmempty = false;

		o = s.option(form.ListValue, 'multiplexing', '多路复用');
		o.value('', '默认');
		o.value('MULTIPLEXING_OFF', '关闭');
		o.value('MULTIPLEXING_LOW', '低');
		o.value('MULTIPLEXING_MIDDLE', '中');
		o.value('MULTIPLEXING_HIGH', '高');
		o.modalonly = true;

		o = s.option(form.ListValue, 'handshake_mode', '握手模式');
		o.value('', '默认');
		o.value('HANDSHAKE_STANDARD', '标准');
		o.value('HANDSHAKE_NO_WAIT', '不等待（0-RTT）');
		o.modalonly = true;

		o = s.option(form.Value, 'mtu', 'MTU', '仅 UDP 传输使用，留空为默认 1400');
		o.datatype = 'range(1280,1500)';
		o.modalonly = true;

		var imp = this.renderImport();
		return m.render().then(function(node) {
			var descr = node.querySelector('.cbi-map-descr');
			node.insertBefore(imp, descr ? descr.nextSibling : node.firstChild);
			return node;
		});
	}
});
