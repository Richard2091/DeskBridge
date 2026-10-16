// 设备页：按连接方式切换，每台设备一行；未登录时显示登录组件。

import 'package:flutter/material.dart';

import '../core/app_model.dart';
import '../core/models.dart';
import '../theme.dart';
import '../widgets/common.dart';

String kindLabel(String kind) => switch (kind) {
      'tailscale' => 'Tailscale',
      'lan' => '局域网',
      'wireguard' => 'WireGuard',
      _ => kind,
    };

String osLabel(String os) => switch (os) {
      'windows' => 'Windows',
      'linux' => 'Linux',
      'darwin' || 'macos' => 'macOS',
      'android' => '安卓',
      'ios' => 'iOS',
      _ => os,
    };

IconData osIcon(String os) => switch (os) {
      'windows' || 'darwin' || 'macos' => Icons.laptop_windows_outlined,
      'linux' => Icons.dns_outlined,
      'android' || 'ios' => Icons.smartphone_outlined,
      _ => Icons.computer_outlined,
    };

class DevicesPage extends StatefulWidget {
  const DevicesPage({super.key, required this.model});
  final AppModel model;

  @override
  State<DevicesPage> createState() => _DevicesPageState();
}

class _DevicesPageState extends State<DevicesPage> {
  String? _kind; // 选中的连接方式
  bool _showOthers = false;

  @override
  Widget build(BuildContext context) {
    final m = widget.model;
    final groups = m.devices?.groups ?? const <DeviceGroup>[];
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 28, 32, 0),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 1040),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            PageTitle('设备', actions: [
              OutlinedButton.icon(
                onPressed: m.devicesLoading ? null : () => m.refreshDevices(force: true),
                icon: m.devicesLoading
                    ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.refresh, size: 18),
                label: const Text('刷新'),
              ),
            ]),
            Expanded(child: _body(m, groups)),
          ]),
        ),
      ),
    );
  }

  Widget _body(AppModel m, List<DeviceGroup> groups) {
    if (m.devices == null) {
      if (m.devicesError != null) {
        return Notice(
          icon: Icons.cloud_off_outlined,
          color: context.tk.bad,
          title: '无法读取设备列表',
          detail: m.devicesError,
          action: FilledButton(onPressed: () => m.refreshDevices(force: true), child: const Text('重试')),
        );
      }
      return const Center(child: CircularProgressIndicator());
    }
    if (groups.isEmpty) {
      return const Notice(icon: Icons.devices_other_outlined, title: '还没有连接方式', detail: '本机节点没有启用任何连接方式。');
    }
    final group = groups.firstWhere((g) => g.kind == _kind, orElse: () => groups.first);
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      if (m.devicesError != null)
        Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: Banner2('刷新设备列表失败：${m.devicesError}', fg: context.tk.bad, bg: context.tk.badSoft, icon: Icons.error_outline),
        ),
      _tabs(groups, group),
      const SizedBox(height: 14),
      Expanded(child: _group(m, group)),
    ]);
  }

  Widget _tabs(List<DeviceGroup> groups, DeviceGroup selected) {
    final tk = context.tk;
    return TabStrip(
      labels: [
        for (final g in groups) '${kindLabel(g.kind)}  ${g.status.connected ? '${g.devices.length}' : '—'}',
      ],
      index: groups.indexOf(selected),
      onChanged: (i) => setState(() => _kind = groups[i].kind),
      trailing: Tooltip(
        message: '添加连接方式（WireGuard 直连、局域网等）将在后续版本支持',
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Icon(Icons.add, size: 18, color: tk.faint),
        ),
      ),
    );
  }

  Widget _group(AppModel m, DeviceGroup g) {
    if (!g.status.connected) return _LoginPanel(model: m, status: g.status);
    final tk = context.tk;
    final main = g.devices.where((d) => d.self || (d.online && d.hasDeskBridge)).toList();
    final others = g.devices.where((d) => !(d.self || (d.online && d.hasDeskBridge))).toList();
    return ListView(padding: const EdgeInsets.only(bottom: 32), children: [
      _statusBar(g),
      const SizedBox(height: 12),
      if (g.error.isNotEmpty)
        Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: Banner2('读取对端列表失败：${g.error}', fg: tk.warn, bg: tk.warnSoft, icon: Icons.warning_amber_outlined),
        ),
      if (main.isEmpty)
        const Padding(padding: EdgeInsets.only(top: 40), child: Notice(icon: Icons.devices_outlined, title: '没有可用的设备', detail: '在其他设备上安装并运行 DeskBridge，用同一个账号登录后会出现在这里。'))
      else
        Panel(
          padding: EdgeInsets.zero,
          child: Column(children: [
            for (var i = 0; i < main.length; i++) ...[
              if (i > 0) Divider(color: tk.border),
              _DeviceRow(device: main[i], onTap: () => m.openDeviceNamed(main[i])),
            ],
          ]),
        ),
      if (others.isNotEmpty) ...[
        const SizedBox(height: 12),
        InkWell(
          borderRadius: BorderRadius.circular(8),
          onTap: () => setState(() => _showOthers = !_showOthers),
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
            child: Row(children: [
              Icon(_showOthers ? Icons.expand_more : Icons.chevron_right, size: 18, color: tk.muted),
              const SizedBox(width: 4),
              Text('还有 ${others.length} 台离线或未安装 DeskBridge 的设备', style: TextStyle(color: tk.muted)),
            ]),
          ),
        ),
        if (_showOthers)
          Panel(
            padding: EdgeInsets.zero,
            child: Column(children: [
              for (var i = 0; i < others.length; i++) ...[
                if (i > 0) Divider(color: tk.border),
                _DeviceRow(device: others[i], onTap: null),
              ],
            ]),
          ),
      ],
    ]);
  }

  Widget _statusBar(DeviceGroup g) {
    final tk = context.tk;
    Device? self;
    for (final d in g.devices) {
      if (d.self) self = d;
    }
    return Row(children: [
      StatusDot(tk.ok),
      const SizedBox(width: 8),
      Text('已连接', style: TextStyle(color: tk.ok, fontWeight: FontWeight.w600)),
      if (g.status.account.isNotEmpty) Text('  ${g.status.account}', style: TextStyle(color: tk.muted)),
      if (self != null && self.addrs.isNotEmpty) Text('  ·  本机 ${self.addrs.first}', style: TextStyle(color: tk.muted)),
    ]);
  }
}

class _DeviceRow extends StatelessWidget {
  const _DeviceRow({required this.device, required this.onTap});
  final Device device;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final d = device;
    final usable = d.self || (d.online && d.hasDeskBridge);
    final muted = !usable;
    String status;
    Color statusColor;
    if (d.self) {
      status = '本机';
      statusColor = tk.accent;
    } else if (!d.online) {
      status = '离线';
      statusColor = tk.faint;
    } else if (!d.hasDeskBridge) {
      status = d.client.isNotEmpty ? '未运行 DeskBridge' : '未安装 DeskBridge';
      statusColor = tk.faint;
    } else if (d.path == 'relay' && d.latencyMs > 150) {
      status = '延迟高';
      statusColor = tk.warn;
    } else {
      status = '在线';
      statusColor = tk.ok;
    }
    return Semantics(
      button: onTap != null,
      label: '${d.displayName}，$status',
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 14),
          child: Row(children: [
            Icon(osIcon(d.os), size: 24, color: muted ? tk.faint : tk.muted),
            const SizedBox(width: 14),
            Expanded(
              flex: 5,
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Flexible(
                    child: Text(d.displayName,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: muted ? tk.muted : tk.text)),
                  ),
                  const SizedBox(width: 10),
                  Row(mainAxisSize: MainAxisSize.min, children: [
                    StatusDot(statusColor, size: 8),
                    const SizedBox(width: 5),
                    Text(status, style: TextStyle(fontSize: 12.5, color: statusColor)),
                  ]),
                ]),
                const SizedBox(height: 3),
                Text(
                  [
                    if (d.os.isNotEmpty) osLabel(d.os),
                    if (d.hasDeskBridge && (d.deskbridgeVersion ?? '').isNotEmpty) 'DeskBridge ${d.deskbridgeVersion}',
                  ].join('  ·  '),
                  style: TextStyle(fontSize: 12.5, color: tk.muted),
                ),
              ]),
            ),
            if (usable && !d.self)
              Expanded(
                flex: 3,
                child: Text(d.pathText, style: TextStyle(fontSize: 13, color: tk.muted)),
              )
            else
              const Spacer(flex: 3),
            SizedBox(
              width: 90,
              child: Text(d.latencyText, textAlign: TextAlign.right, style: TextStyle(fontSize: 13, color: tk.muted)),
            ),
            SizedBox(width: 24, child: onTap == null ? null : Icon(Icons.chevron_right, color: tk.faint)),
          ]),
        ),
      ),
    );
  }
}

/// 连接方式没有连上时的组件：未登录给登录入口，其他情况说明原因。
class _LoginPanel extends StatefulWidget {
  const _LoginPanel({required this.model, required this.status});
  final AppModel model;
  final ChannelStatus status;

  @override
  State<_LoginPanel> createState() => _LoginPanelState();
}

class _LoginPanelState extends State<_LoginPanel> {
  bool _busy = false;
  String? _error;
  String _url = '';

  Future<void> _login() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final st = await widget.model.api.login(widget.status.kind);
      _url = st.loginUrl;
      if (_url.isNotEmpty) await openUrl(_url);
      await widget.model.refreshDevices(force: true);
    } on ApiException catch (e) {
      _error = e.message;
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final st = widget.status;
    final tk = context.tk;
    final url = _url.isNotEmpty ? _url : st.loginUrl;
    final String title;
    final String? detail;
    switch (st.state) {
      case 'needs_login':
        title = '登录 ${kindLabel(st.kind)}';
        detail = '登录后，同一账号下运行 DeskBridge 的设备会出现在这里。';
      case 'starting':
        title = '${kindLabel(st.kind)} 正在启动';
        detail = st.reason.isNotEmpty ? st.reason : '请稍候。';
      case 'disconnected':
        title = '${kindLabel(st.kind)} 已断开';
        detail = st.reason.isNotEmpty ? st.reason : '正在重新连接。';
      default:
        title = '${kindLabel(st.kind)} 未启用';
        detail = st.reason.isNotEmpty ? st.reason : null;
    }
    return Notice(
      icon: st.needsLogin ? Icons.login : Icons.link_off,
      color: st.state == 'disconnected' ? tk.bad : tk.accent,
      title: title,
      detail: detail,
      action: Column(mainAxisSize: MainAxisSize.min, children: [
        if (st.needsLogin || st.state == 'stopped')
          FilledButton(onPressed: _busy ? null : _login, child: Text(_busy ? '请稍候…' : '登录')),
        if (url.isNotEmpty) ...[
          const SizedBox(height: 12),
          Text('如果浏览器没有自动打开，请复制下面的链接到浏览器：', style: TextStyle(color: tk.muted, fontSize: 12.5)),
          const SizedBox(height: 6),
          Row(mainAxisSize: MainAxisSize.min, children: [
            Flexible(child: SelectableText(url, style: monoStyle(context, size: 12, color: tk.info))),
            IconButton(tooltip: '复制链接', onPressed: () => copyText(context, url, done: '已复制登录链接'), icon: const Icon(Icons.copy, size: 16)),
          ]),
        ],
        if (_error != null) ...[
          const SizedBox(height: 10),
          Text(_error!, style: TextStyle(color: tk.bad)),
        ],
      ]),
    );
  }
}
