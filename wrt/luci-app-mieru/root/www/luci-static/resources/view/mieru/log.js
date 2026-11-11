'use strict';
'require view';
'require fs';
'require poll';
'require ui';

function fetchLog() {
	return fs.exec('/sbin/logread', [ '-e', 'mieru' ]).then(function(res) {
		var lines = (res.stdout || '').trim().split('\n');
		return lines.slice(-500).join('\n') || '（暂无日志）';
	}).catch(function(e) { return '读取日志失败：' + e.message; });
}

return view.extend({
	load: fetchLog,

	render: function(text) {
		var ta = E('textarea', {
			'class': 'cbi-input-textarea',
			'style': 'width:100%;font-family:monospace;font-size:12px',
			'rows': 30,
			'readonly': 'readonly',
			'wrap': 'off'
		}, [ text ]);

		var follow = E('input', { 'type': 'checkbox', 'checked': 'checked' });

		poll.add(function() {
			return fetchLog().then(function(t) {
				ta.value = t;
				if (follow.checked) ta.scrollTop = ta.scrollHeight;
			});
		}, 5);

		window.setTimeout(function() { ta.scrollTop = ta.scrollHeight; }, 0);

		return E('div', { 'class': 'cbi-map' }, [
			E('h2', {}, 'Mieru 日志'),
			E('div', { 'class': 'cbi-map-descr' }, '来自系统日志（logread），每 5 秒刷新，显示最近 500 行。'),
			E('div', { 'class': 'cbi-section' }, [
				E('label', { 'style': 'display:block;margin-bottom:.5em' }, [ follow, ' 自动滚动到底部' ]),
				ta
			])
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
