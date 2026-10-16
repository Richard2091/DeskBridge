// 设备页：头部、AI 进行中的提示条、概览 / 终端 / 文件三个标签。

import 'dart:async';

import 'package:flutter/material.dart';

import '../core/api.dart';
import '../core/app_model.dart';
import '../core/models.dart';
import '../core/output.dart';
import '../theme.dart';
import '../widgets/common.dart';
import '../widgets/output_view.dart';
import 'devices_page.dart';

class DevicePage extends StatefulWidget {
  const DevicePage({super.key, required this.model, required this.device});
  final AppModel model;
  final Device device;

  @override
  State<DevicePage> createState() => _DevicePageState();
}

class _DevicePageState extends State<DevicePage> {
  int _tab = 0;

  @override
  Widget build(BuildContext context) {
    final m = widget.model;
    final d = widget.device;
    final tk = context.tk;
    final usable = d.self || (d.online && d.hasDeskBridge);
    final aiRunning = (m.activity?.items ?? const <ActivityItem>[])
        .where((e) => e.running && e.isAi && e.role == 'target' && e.targetRef == d.target)
        .toList();
    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 24, 32, 0),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 1100),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            _header(context, d),
            const SizedBox(height: 14),
            for (final a in aiRunning.take(2))
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Banner2(
                  'AI 正在这台设备上执行 ${a.tool}：${_short(a.args)}',
                  fg: tk.ai,
                  bg: tk.aiSoft,
                  icon: Icons.smart_toy_outlined,
                  action: TextButton(onPressed: () => m.openActivityItem(a.requestId), child: const Text('查看')),
                ),
              ),
            if (!usable)
              Expanded(
                child: Notice(
                  icon: Icons.link_off,
                  title: !d.online ? '设备离线' : '这台设备没有运行 DeskBridge',
                  detail: !d.online ? '设备上线后才能执行命令和浏览文件。' : '在这台设备上安装并运行 DeskBridge 后，才能执行命令和浏览文件。',
                ),
              )
            else ...[
              TabStrip(labels: const ['概览', '终端', '文件'], index: _tab, onChanged: (i) => setState(() => _tab = i)),
              Expanded(
                child: IndexedStack(index: _tab, children: [
                  _OverviewTab(model: m, device: d),
                  _TerminalTab(model: m, device: d),
                  _FilesTab(model: m, device: d),
                ]),
              ),
            ],
          ]),
        ),
      ),
    );
  }

  Widget _header(BuildContext context, Device d) {
    final tk = context.tk;
    final status = d.self ? '本机' : (!d.online ? '离线' : (!d.hasDeskBridge ? '未运行 DeskBridge' : '在线'));
    final statusColor = d.self ? tk.accent : (d.online && d.hasDeskBridge ? tk.ok : tk.faint);
    return Row(crossAxisAlignment: CrossAxisAlignment.center, children: [
      IconButton(tooltip: '返回设备列表', onPressed: widget.model.closeDevice, icon: const Icon(Icons.arrow_back)),
      const SizedBox(width: 4),
      Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(d.displayName, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w600)),
          const SizedBox(height: 4),
          Row(children: [
            StatusDot(statusColor, size: 8),
            const SizedBox(width: 6),
            Text(status, style: TextStyle(color: statusColor, fontSize: 13)),
            Text(
              [
                '',
                kindLabel(d.kind),
                if (!d.self && d.online) d.pathText,
                if (d.latencyText.isNotEmpty) d.latencyText,
                if (d.os.isNotEmpty) osLabel(d.os),
              ].join('  ·  '),
              style: TextStyle(color: tk.muted, fontSize: 13),
            ),
          ]),
        ]),
      ),
      Tooltip(
        message: '需要桌面工具，后续版本支持',
        child: OutlinedButton.icon(onPressed: null, icon: const Icon(Icons.desktop_windows_outlined, size: 18), label: const Text('远程桌面')),
      ),
    ]);
  }
}

String _short(String s) => s.length > 80 ? '${s.substring(0, 80)}…' : s;

// ───────────────────────── 概览 ─────────────────────────

class _OverviewTab extends StatefulWidget {
  const _OverviewTab({required this.model, required this.device});
  final AppModel model;
  final Device device;
  @override
  State<_OverviewTab> createState() => _OverviewTabState();
}

class _OverviewTabState extends State<_OverviewTab> {
  Map<String, dynamic>? _info;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final r = await widget.model.api.callTool(widget.device.target, 'sys.info', null, timeoutMs: 20000);
      if (mounted) setState(() => _info = r.result);
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final m = widget.model;
    final d = widget.device;
    final recent = (m.activity?.items ?? const <ActivityItem>[])
        .where((e) => e.targetRef == d.target || (e.targetRef.isEmpty && e.targetName == d.displayName))
        .take(5)
        .toList();
    return ListView(padding: const EdgeInsets.only(top: 20, bottom: 32), children: [
      const SectionTitle('设备信息'),
      Panel(child: _infoBody(context)),
      const SizedBox(height: 24),
      const SectionTitle('最近操作'),
      Panel(
        padding: EdgeInsets.zero,
        child: recent.isEmpty
            ? Padding(padding: const EdgeInsets.all(20), child: Text('暂无操作', style: TextStyle(color: tk.muted)))
            : Column(children: [
                for (var i = 0; i < recent.length; i++) ...[
                  if (i > 0) Divider(color: tk.border),
                  ActivityRow(item: recent[i], onTap: () => m.openActivityItem(recent[i].requestId), showTarget: false),
                ],
              ]),
      ),
    ]);
  }

  Widget _infoBody(BuildContext context) {
    final tk = context.tk;
    if (_error != null) return Text('读取设备信息失败：$_error', style: TextStyle(color: tk.bad));
    final i = _info;
    if (i == null) return const Center(child: Padding(padding: EdgeInsets.all(12), child: CircularProgressIndicator()));
    String s(String k) => '${i[k] ?? ''}';
    String mem() {
      final total = i['memory_total_bytes'], avail = i['memory_available_bytes'];
      if (total is! num) return '';
      final t = formatBytes(total.toInt());
      return avail is num ? '$t（可用 ${formatBytes(avail.toInt())}）' : t;
    }

    final rows = <(String, String)>[
      ('设备名称', s('name')),
      ('主机名', s('hostname')),
      ('系统', [s('os'), s('arch'), s('os_version')].where((e) => e.isNotEmpty).join(' ')),
      ('用户', s('user')),
      ('CPU 核数', s('num_cpu')),
      ('内存', mem()),
      ('时区', s('timezone')),
      ('DeskBridge 版本', s('version')),
      ('用户目录', s('home')),
    ].where((e) => e.$2.isNotEmpty).toList();
    return Wrap(runSpacing: 14, spacing: 32, children: [
      for (final r in rows)
        SizedBox(
          width: 300,
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(r.$1, style: TextStyle(fontSize: 12.5, color: tk.muted)),
            const SizedBox(height: 3),
            SelectableText(r.$2, style: const TextStyle(fontSize: 14)),
          ]),
        ),
    ]);
  }
}

// ───────────────────────── 终端 ─────────────────────────

class _TerminalTab extends StatefulWidget {
  const _TerminalTab({required this.model, required this.device});
  final AppModel model;
  final Device device;
  @override
  State<_TerminalTab> createState() => _TerminalTabState();
}

class _TerminalTabState extends State<_TerminalTab> {
  final _cmd = TextEditingController();
  final _cwd = TextEditingController();
  final _focus = FocusNode();
  final _out = OutputController();
  final List<String> _history = [];
  bool _running = false;
  bool _showOptions = false;
  String? _requestId;
  DateTime? _startedAt;
  Timer? _tick;
  String _summary = ''; // 上一次的结果摘要
  Color? _summaryColor;

  @override
  void dispose() {
    _tick?.cancel();
    _cmd.dispose();
    _cwd.dispose();
    _focus.dispose();
    _out.dispose();
    super.dispose();
  }

  Future<void> _run() async {
    final command = _cmd.text.trim();
    if (command.isEmpty || _running) return;
    final api = widget.model.api;
    final target = widget.device.target;
    final id = Api.newRequestId();
    _history.remove(command);
    _history.insert(0, command);
    if (_history.length > 20) _history.removeLast();
    _out.clear();
    setState(() {
      _running = true;
      _requestId = id;
      _startedAt = DateTime.now();
      _summary = '';
    });
    _tick = Timer.periodic(const Duration(seconds: 1), (_) => setState(() {}));
    // 实时输出：调用刚发出时节点可能还没登记，订阅会重试。
    unawaited(_out.follow(api, target, id, maxTries: 25, giveUp: () => !_running));
    final tk = context.tk;
    try {
      final r = await api.callTool(
        target,
        'proc.exec',
        {'command': command, if (_cwd.text.trim().isNotEmpty) 'cwd': _cwd.text.trim()},
        requestId: id,
        timeoutMs: 600000,
      );
      final res = r.result;
      if (!_out.hasOutput) {
        // 命令太快，实时订阅没赶上：直接显示结果里的输出。
        final so = '${res['stdout'] ?? ''}', se = '${res['stderr'] ?? ''}';
        if (so.isNotEmpty) _out.apply(CallEvent(type: 'output', stream: 'stdout', data: so));
        if (se.isNotEmpty) _out.apply(CallEvent(type: 'output', stream: 'stderr', data: se));
      }
      final code = res['exit_code'];
      final timedOut = res['timed_out'] == true;
      _summary = timedOut
          ? '命令超时 · 耗时 ${formatDuration(Duration(milliseconds: r.durationMs))}'
          : '退出码 $code · 耗时 ${formatDuration(Duration(milliseconds: r.durationMs))}';
      _summaryColor = (timedOut || code != 0) ? tk.bad : tk.ok;
    } on ApiException catch (e) {
      _summary = e.code == 'cancelled' ? '已取消' : '失败：${e.message}';
      _summaryColor = e.code == 'cancelled' ? tk.warn : tk.bad;
    } finally {
      _tick?.cancel();
      if (mounted) {
        setState(() => _running = false);
        _focus.requestFocus();
      }
    }
  }

  Future<void> _cancel() async {
    final id = _requestId;
    if (id == null) return;
    try {
      await widget.model.api.cancel(widget.device.target, id);
    } on ApiException catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('取消失败：${e.message}')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final elapsed = _startedAt == null ? '' : formatDuration(DateTime.now().difference(_startedAt!));
    return Padding(
      padding: const EdgeInsets.only(top: 20, bottom: 24),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Expanded(
            child: TextField(
              controller: _cmd,
              focusNode: _focus,
              enabled: !_running,
              autofocus: true,
              style: monoStyle(context),
              decoration: InputDecoration(
                hintText: '输入命令，回车执行',
                prefixIcon: Icon(Icons.chevron_right, color: tk.accent),
                hintStyle: TextStyle(color: tk.faint),
              ),
              onSubmitted: (_) => _run(),
            ),
          ),
          const SizedBox(width: 10),
          if (_running)
            FilledButton.icon(
              style: FilledButton.styleFrom(backgroundColor: tk.bad),
              onPressed: _cancel,
              icon: const Icon(Icons.stop, size: 18),
              label: const Text('取消'),
            )
          else
            FilledButton.icon(onPressed: _run, icon: const Icon(Icons.play_arrow, size: 18), label: const Text('执行')),
          const SizedBox(width: 6),
          IconButton(
            tooltip: '选项和历史',
            onPressed: () => setState(() => _showOptions = !_showOptions),
            icon: Icon(_showOptions ? Icons.expand_less : Icons.tune),
          ),
        ]),
        if (_showOptions) ...[
          const SizedBox(height: 10),
          Panel(
            padding: const EdgeInsets.all(14),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              TextField(controller: _cwd, decoration: const InputDecoration(labelText: '工作目录（留空用节点的当前目录）')),
              if (_history.isNotEmpty) ...[
                const SizedBox(height: 12),
                Text('历史', style: TextStyle(fontSize: 12.5, color: tk.muted)),
                const SizedBox(height: 6),
                Wrap(spacing: 8, runSpacing: 6, children: [
                  for (final h in _history.take(10))
                    ActionChip(
                      label: Text(_short(h), style: monoStyle(context, size: 12)),
                      onPressed: _running ? null : () => setState(() => _cmd.text = h),
                    ),
                ]),
              ],
            ]),
          ),
        ],
        const SizedBox(height: 12),
        Expanded(child: OutputView(controller: _out, height: double.infinity, placeholder: '在上面输入命令并执行，输出会实时显示在这里。')),
        const SizedBox(height: 8),
        SizedBox(
          height: 22,
          child: _running
              ? Row(children: [
                  const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)),
                  const SizedBox(width: 8),
                  Text('运行中 · $elapsed', style: TextStyle(color: tk.info)),
                ])
              : Text(_summary, style: TextStyle(color: _summaryColor ?? tk.muted)),
        ),
      ]),
    );
  }
}

// ───────────────────────── 文件 ─────────────────────────

class _FilesTab extends StatefulWidget {
  const _FilesTab({required this.model, required this.device});
  final AppModel model;
  final Device device;
  @override
  State<_FilesTab> createState() => _FilesTabState();
}

class _FilesTabState extends State<_FilesTab> {
  final _pathCtl = TextEditingController();
  String _path = '';
  List<FileEntry> _entries = [];
  bool _truncated = false;
  bool _loading = false;
  String? _error;
  bool _started = false;
  // 预览
  String? _previewName;
  String? _previewText;
  String? _previewNote;
  bool _previewLoading = false;

  @override
  void initState() {
    super.initState();
    _init();
  }

  @override
  void dispose() {
    _pathCtl.dispose();
    super.dispose();
  }

  Future<void> _init() async {
    try {
      final r = await widget.model.api.callTool(widget.device.target, 'sys.info', null, timeoutMs: 20000);
      final home = '${r.result['home'] ?? ''}';
      await _go(home.isNotEmpty ? home : (widget.device.os == 'windows' ? 'C:\\' : '/'));
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    }
  }

  bool get _win => _path.contains('\\') || RegExp(r'^[A-Za-z]:').hasMatch(_path) || widget.device.os == 'windows';
  String get _sep => _win ? '\\' : '/';

  String _join(String dir, String name) => dir.endsWith(_sep) ? '$dir$name' : '$dir$_sep$name';

  String _parent(String p) {
    var s = p;
    while (s.length > 1 && s.endsWith(_sep)) {
      s = s.substring(0, s.length - 1);
    }
    final i = s.lastIndexOf(_sep);
    if (i < 0) return p;
    if (i == 0) return _sep;
    final up = s.substring(0, i);
    return (_win && up.length == 2 && up.endsWith(':')) ? '$up\\' : up;
  }

  Future<void> _go(String path) async {
    setState(() {
      _loading = true;
      _error = null;
      _started = true;
    });
    try {
      final r = await widget.model.api.callTool(widget.device.target, 'fs.list', {'path': path, 'max_entries': 2000}, timeoutMs: 30000);
      final list = (r.result['entries'] as List? ?? const []).whereType<Map<String, dynamic>>().map(FileEntry.fromJson).toList();
      list.sort((a, b) {
        if (a.isDir != b.isDir) return a.isDir ? -1 : 1;
        return a.name.toLowerCase().compareTo(b.name.toLowerCase());
      });
      if (!mounted) return;
      setState(() {
        _path = '${r.result['path'] ?? path}';
        _pathCtl.text = _path;
        _entries = list;
        _truncated = r.result['truncated'] == true;
        _previewName = null;
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.code == 'permission_denied' ? '系统拒绝访问：${e.message}' : e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _preview(FileEntry f) async {
    setState(() {
      _previewName = f.name;
      _previewText = null;
      _previewNote = null;
      _previewLoading = true;
    });
    try {
      final r = await widget.model.api.callTool(
        widget.device.target,
        'fs.read',
        {'path': _join(_path, f.name), 'max_bytes': 65536, 'encoding': 'auto'},
        timeoutMs: 30000,
      );
      if (!mounted) return;
      setState(() {
        if (r.result['encoding'] == 'base64') {
          _previewNote = '二进制文件，不预览（${formatBytes(f.size)}）';
        } else {
          _previewText = '${r.result['content'] ?? ''}';
          if (r.result['eof'] != true) _previewNote = '只显示前 64 KB（共 ${formatBytes(f.size)}）';
        }
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _previewNote = '读取失败：${e.message}');
    } finally {
      if (mounted) setState(() => _previewLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Padding(
      padding: const EdgeInsets.only(top: 20, bottom: 24),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          IconButton(tooltip: '上一级', onPressed: _loading || _path.isEmpty ? null : () => _go(_parent(_path)), icon: const Icon(Icons.arrow_upward)),
          Expanded(
            child: TextField(
              controller: _pathCtl,
              style: monoStyle(context, size: 13),
              decoration: const InputDecoration(hintText: '路径，回车跳转'),
              onSubmitted: (v) => v.trim().isEmpty ? null : _go(v.trim()),
            ),
          ),
          IconButton(tooltip: '刷新', onPressed: _loading || _path.isEmpty ? null : () => _go(_path), icon: const Icon(Icons.refresh)),
        ]),
        const SizedBox(height: 12),
        if (_error != null)
          Banner2(_error!, fg: tk.bad, bg: tk.badSoft, icon: Icons.error_outline),
        if (_truncated)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Banner2('目录条目太多，只显示前 2000 项', fg: tk.warn, bg: tk.warnSoft, icon: Icons.warning_amber_outlined),
          ),
        Expanded(
          child: !_started && _error == null
              ? const Center(child: CircularProgressIndicator())
              : Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Expanded(
                    flex: 3,
                    child: Panel(
                      padding: EdgeInsets.zero,
                      child: _loading
                          ? const Center(child: CircularProgressIndicator())
                          : _entries.isEmpty
                              ? Notice(icon: Icons.folder_open_outlined, title: _error == null ? '空文件夹' : '无法显示', detail: null)
                              : ListView.builder(
                                  itemCount: _entries.length,
                                  itemBuilder: (ctx, i) => _entryRow(ctx, _entries[i]),
                                ),
                    ),
                  ),
                  if (_previewName != null) ...[
                    const SizedBox(width: 14),
                    Expanded(flex: 2, child: _previewPanel(context)),
                  ],
                ]),
        ),
      ]),
    );
  }

  Widget _entryRow(BuildContext context, FileEntry e) {
    final tk = context.tk;
    final selected = e.name == _previewName;
    return InkWell(
      onTap: () => e.isDir ? _go(_join(_path, e.name)) : _preview(e),
      child: Container(
        color: selected ? tk.accentSoft : null,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 9),
        child: Row(children: [
          Icon(e.isDir ? Icons.folder_outlined : Icons.insert_drive_file_outlined, size: 20, color: e.isDir ? tk.accent : tk.muted),
          const SizedBox(width: 12),
          Expanded(child: Text(e.name, overflow: TextOverflow.ellipsis)),
          SizedBox(width: 90, child: Text(e.isDir ? '' : formatBytes(e.size), textAlign: TextAlign.right, style: TextStyle(color: tk.muted, fontSize: 12.5))),
          SizedBox(width: 150, child: Text(_mtime(e.mtime), textAlign: TextAlign.right, style: TextStyle(color: tk.muted, fontSize: 12.5))),
        ]),
      ),
    );
  }

  String _mtime(String s) {
    final t = DateTime.tryParse(s)?.toLocal();
    if (t == null) return s;
    return '${t.year}-${two(t.month)}-${two(t.day)} ${two(t.hour)}:${two(t.minute)}';
  }

  Widget _previewPanel(BuildContext context) {
    final tk = context.tk;
    return Panel(
      padding: const EdgeInsets.all(14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Expanded(child: Text(_previewName ?? '', style: const TextStyle(fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis)),
          IconButton(tooltip: '关闭预览', onPressed: () => setState(() => _previewName = null), icon: const Icon(Icons.close, size: 18)),
        ]),
        if (_previewNote != null) Padding(padding: const EdgeInsets.only(bottom: 8), child: Text(_previewNote!, style: TextStyle(color: tk.muted, fontSize: 12.5))),
        Expanded(
          child: _previewLoading
              ? const Center(child: CircularProgressIndicator())
              : _previewText == null
                  ? const SizedBox()
                  : CodeBox(
                      child: SingleChildScrollView(
                        child: SizedBox(width: double.infinity, child: SelectableText(_previewText!, style: monoStyle(context, size: 12.5, color: tk.codeText))),
                      ),
                    ),
        ),
      ]),
    );
  }
}

/// 操作记录的一行（概览的“最近操作”和操作记录页共用）。
class ActivityRow extends StatelessWidget {
  const ActivityRow({super.key, required this.item, required this.onTap, this.showTarget = true, this.selected = false});
  final ActivityItem item;
  final VoidCallback onTap;
  final bool showTarget;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final it = item;
    final (String label, Color fg, Color bg) = it.running
        ? ('进行中', tk.info, tk.infoSoft)
        : it.ok
            ? ('成功', tk.ok, tk.okSoft)
            : it.cancelled
                ? ('已取消', tk.warn, tk.warnSoft)
                : it.status == 'interrupted'
                    ? ('已中断', tk.warn, tk.warnSoft)
                    : ('失败 · ${it.status}', tk.bad, tk.badSoft);
    return Semantics(
      button: true,
      label: '${it.isAi ? 'AI' : '人'} ${it.tool} $label',
      child: InkWell(
        onTap: onTap,
        child: Container(
          color: selected ? tk.accentSoft : null,
          padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
          child: Row(children: [
            Pill(it.isAi ? 'AI' : '人', fg: it.isAi ? tk.ai : tk.muted, bg: it.isAi ? tk.aiSoft : tk.surface3, icon: it.isAi ? Icons.smart_toy_outlined : Icons.person_outline),
            const SizedBox(width: 12),
            SizedBox(width: 78, child: Text(it.tool, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13))),
            if (showTarget) ...[
              SizedBox(width: 90, child: Text(it.targetName.isEmpty ? '—' : it.targetName, overflow: TextOverflow.ellipsis, style: TextStyle(color: tk.muted, fontSize: 13))),
              const SizedBox(width: 8),
            ],
            Expanded(child: Text(_argSummary(it), overflow: TextOverflow.ellipsis, style: monoStyle(context, size: 12, color: tk.muted))),
            const SizedBox(width: 10),
            Pill(label, fg: fg, bg: bg),
            SizedBox(width: 74, child: Text(clock(it.startedAt), textAlign: TextAlign.right, style: TextStyle(color: tk.faint, fontSize: 12.5))),
          ]),
        ),
      ),
    );
  }
}

/// 把参数摘要里最有用的字段（命令、路径）提出来显示。
String _argSummary(ActivityItem it) {
  final s = it.args;
  if (s.isEmpty) return '';
  final m = RegExp(r'"(command|path)":"((?:[^"\\]|\\.)*)"').firstMatch(s);
  if (m != null) return m.group(2)!.replaceAll(r'\\', r'\').replaceAll(r'\"', '"');
  return s;
}
