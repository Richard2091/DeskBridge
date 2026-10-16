// 设置：本机、连接方式、AI 接入、外观、关于（基础版）。

import 'package:flutter/material.dart';

import '../core/app_model.dart';
import '../core/models.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'devices_page.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key, required this.model});
  final AppModel model;
  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  int _section = 0;
  static const _sections = ['本机', '连接方式', 'AI 接入', '外观', '关于'];

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 28, 32, 0),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 860),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const PageTitle('设置'),
            Expanded(
              child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                SizedBox(
                  width: 150,
                  child: Column(children: [
                    for (var i = 0; i < _sections.length; i++)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 2),
                        child: Material(
                          color: i == _section ? tk.accentSoft : Colors.transparent,
                          borderRadius: BorderRadius.circular(8),
                          child: InkWell(
                            borderRadius: BorderRadius.circular(8),
                            onTap: () => setState(() => _section = i),
                            child: Container(
                              width: double.infinity,
                              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
                              child: Text(_sections[i], style: TextStyle(fontWeight: i == _section ? FontWeight.w600 : FontWeight.w400, color: i == _section ? tk.accent : tk.text)),
                            ),
                          ),
                        ),
                      ),
                  ]),
                ),
                const SizedBox(width: 24),
                Expanded(child: ListView(padding: const EdgeInsets.only(bottom: 32), children: [_content(context)])),
              ]),
            ),
          ]),
        ),
      ),
    );
  }

  Widget _content(BuildContext context) {
    final m = widget.model;
    return switch (_section) {
      0 => _local(context, m),
      1 => _channels(context, m),
      2 => _ai(context),
      3 => _appearance(context, m),
      _ => _about(context, m),
    };
  }

  Widget _kv(BuildContext context, String k, String v, {bool mono = false}) {
    final tk = context.tk;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        SizedBox(width: 110, child: Text(k, style: TextStyle(color: tk.muted))),
        Expanded(child: SelectableText(v.isEmpty ? '—' : v, style: mono ? monoStyle(context, size: 12.5) : null)),
      ]),
    );
  }

  Widget _local(BuildContext context, AppModel m) {
    final st = m.status;
    if (st == null) return Notice(icon: Icons.cloud_off_outlined, title: '读不到本机节点', detail: m.devicesError ?? '正在连接…');
    return Panel(
      child: Column(children: [
        _kv(context, '设备名称', st.name),
        _kv(context, '设备 ID', st.deviceId, mono: true),
        _kv(context, '节点版本', st.version),
        Align(
          alignment: Alignment.centerLeft,
          child: Padding(
            padding: const EdgeInsets.only(top: 10),
            child: Text('设备名称、开机自启等用 deskbridge node / install 的参数设置（deskbridge install -h 查看）。', style: TextStyle(color: context.tk.muted, fontSize: 12.5)),
          ),
        ),
      ]),
    );
  }

  Widget _channels(BuildContext context, AppModel m) {
    final tk = context.tk;
    final chans = m.status?.channels ?? const <ChannelStatus>[];
    if (chans.isEmpty) return const Notice(icon: Icons.link_off, title: '没有连接方式信息');
    return Column(children: [
      for (final c in chans)
        Padding(
          padding: const EdgeInsets.only(bottom: 14),
          child: Panel(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(children: [
                Text(kindLabel(c.kind), style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                const SizedBox(width: 10),
                StatusDot(c.connected ? tk.ok : (c.state == 'disconnected' ? tk.bad : tk.faint), size: 8),
                const SizedBox(width: 5),
                Text(_stateText(c.state), style: TextStyle(color: tk.muted)),
              ]),
              const SizedBox(height: 8),
              _kv(context, '账号', c.account),
              if (c.reason.isNotEmpty) _kv(context, '说明', c.reason),
              const SizedBox(height: 10),
              Row(children: [
                if (c.connected || c.state == 'disconnected')
                  OutlinedButton(
                    onPressed: () async {
                      final ok = await confirm(context,
                          title: '退出 ${kindLabel(c.kind)} 登录？', body: '退出后本机不再加入这个网络，需要重新登录才能访问其中的设备。', ok: '退出登录', danger: true);
                      if (!ok) return;
                      try {
                        await m.api.logout(c.kind);
                        await m.refreshStatus();
                        await m.refreshDevices(force: true);
                      } on ApiException catch (e) {
                        if (context.mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('退出登录失败：${e.message}')));
                      }
                    },
                    child: const Text('退出登录'),
                  )
                else
                  FilledButton(
                    onPressed: () {
                      m.goTo(AppPage.devices);
                    },
                    child: const Text('去设备页登录'),
                  ),
              ]),
            ]),
          ),
        ),
    ]);
  }

  String _stateText(String s) => switch (s) {
        'connected' => '已连接',
        'needs_login' => '需要登录',
        'disconnected' => '已断开',
        'starting' => '正在启动',
        'stopped' => '未启动',
        _ => s,
      };

  Widget _ai(BuildContext context) {
    const snippet = '''{
  "mcpServers": {
    "deskbridge": {
      "command": "deskbridge",
      "args": ["mcp"]
    }
  }
}''';
    final tk = context.tk;
    return Panel(
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        const Text('让 AI 控制其他设备', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
        const SizedBox(height: 8),
        Text('在 Claude Code 等支持 MCP 的 AI 客户端里加入下面的配置（需要 deskbridge 在 PATH 中，本机节点已运行）。AI 的每次操作都会记在“操作记录”里，你可以随时查看和取消。', style: TextStyle(color: tk.muted, height: 1.5)),
        const SizedBox(height: 14),
        CodeBox(child: SelectableText(snippet, style: monoStyle(context, size: 12.5, color: tk.codeText))),
        const SizedBox(height: 10),
        OutlinedButton.icon(onPressed: () => copyText(context, snippet, done: '已复制 MCP 配置'), icon: const Icon(Icons.copy, size: 16), label: const Text('复制配置')),
      ]),
    );
  }

  Widget _appearance(BuildContext context, AppModel m) {
    return Panel(
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        const Text('主题', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
        const SizedBox(height: 12),
        SegmentedButton<ThemeMode>(
          showSelectedIcon: false,
          segments: const [
            ButtonSegment(value: ThemeMode.system, label: Text('跟随系统')),
            ButtonSegment(value: ThemeMode.light, label: Text('浅色')),
            ButtonSegment(value: ThemeMode.dark, label: Text('深色')),
          ],
          selected: {m.themeMode},
          onSelectionChanged: (s) => m.setThemeMode(s.first),
        ),
        const SizedBox(height: 10),
        Text('基础版只有“清爽”主题；“黑白”“柔光”后续再做。', style: TextStyle(color: context.tk.muted, fontSize: 12.5)),
      ]),
    );
  }

  Widget _about(BuildContext context, AppModel m) {
    return Panel(
      child: Column(children: [
        _kv(context, '产品', 'DeskBridge 客户端（基础版）'),
        _kv(context, '节点版本', m.status?.version ?? ''),
        _kv(context, '说明', '设备列表、终端、文件浏览和操作记录。截图和远程桌面需要桌面工具，后续版本支持。'),
      ]),
    );
  }
}
