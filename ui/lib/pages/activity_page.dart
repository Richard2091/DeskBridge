// 操作记录页：合并各设备的记录，进行中的排在最前，点开看详情、实时输出和取消。

import 'dart:async';

import 'package:flutter/material.dart';

import '../core/app_model.dart';
import '../core/models.dart';
import '../core/output.dart';
import '../theme.dart';
import '../widgets/common.dart';
import '../widgets/output_view.dart';
import 'device_page.dart';

class ActivityPage extends StatefulWidget {
  const ActivityPage({super.key, required this.model});
  final AppModel model;
  @override
  State<ActivityPage> createState() => _ActivityPageState();
}

class _ActivityPageState extends State<ActivityPage> {
  String _filter = ''; // '' 全部 / ai / human

  @override
  Widget build(BuildContext context) {
    final m = widget.model;
    final tk = context.tk;
    final res = m.activity;
    final all = (res?.items ?? const <ActivityItem>[]).where((e) => _filter.isEmpty || e.operatorType == _filter).toList();
    // 进行中的在最前，其余按时间从新到旧（Go 端已排好序，这里只把进行中的提到前面）。
    final running = all.where((e) => e.running).toList();
    final rest = all.where((e) => !e.running).toList();
    final selected = m.selectedRequestId == null
        ? null
        : (res?.items ?? const <ActivityItem>[]).cast<ActivityItem?>().firstWhere((e) => e!.requestId == m.selectedRequestId, orElse: () => null);

    return Padding(
      padding: const EdgeInsets.fromLTRB(32, 28, 32, 0),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Expanded(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1040),
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                PageTitle('操作记录', actions: [
                  SegmentedButton<String>(
                    showSelectedIcon: false,
                    style: SegmentedButton.styleFrom(visualDensity: VisualDensity.compact),
                    segments: const [
                      ButtonSegment(value: '', label: Text('全部')),
                      ButtonSegment(value: 'ai', label: Text('AI')),
                      ButtonSegment(value: 'human', label: Text('人')),
                    ],
                    selected: {_filter},
                    onSelectionChanged: (s) => setState(() => _filter = s.first),
                  ),
                ]),
                if (res != null && res.errors.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: Banner2(
                      res.errors.entries.map((e) => '${e.key}：${e.value}').join('；'),
                      fg: tk.warn,
                      bg: tk.warnSoft,
                      icon: Icons.warning_amber_outlined,
                    ),
                  ),
                if (m.activityError != null && res == null)
                  Expanded(
                    child: Notice(
                      icon: Icons.error_outline,
                      color: tk.bad,
                      title: '无法读取操作记录',
                      detail: m.activityError,
                      action: FilledButton(onPressed: m.refreshActivity, child: const Text('重试')),
                    ),
                  )
                else if (res == null)
                  const Expanded(child: Center(child: CircularProgressIndicator()))
                else if (all.isEmpty)
                  Expanded(
                    child: Notice(
                      icon: Icons.history,
                      title: _filter.isEmpty ? '还没有操作记录' : '没有符合条件的记录',
                      detail: _filter.isEmpty ? 'AI 或你在任何设备上执行的操作会记录在这里。' : null,
                      action: _filter.isEmpty ? null : TextButton(onPressed: () => setState(() => _filter = ''), child: const Text('清除筛选')),
                    ),
                  )
                else
                  Expanded(child: _list(context, running, rest)),
              ]),
            ),
          ),
        ),
        if (selected != null) ...[
          const SizedBox(width: 20),
          SizedBox(
            width: 420,
            child: Padding(
              padding: const EdgeInsets.only(bottom: 24),
              child: _DetailPanel(key: ValueKey(selected.requestId), model: m, item: selected),
            ),
          ),
        ],
      ]),
    );
  }

  Widget _list(BuildContext context, List<ActivityItem> running, List<ActivityItem> rest) {
    final tk = context.tk;
    final m = widget.model;
    final children = <Widget>[];
    if (running.isNotEmpty) {
      children.add(const SectionTitle('进行中'));
      children.add(_group(running));
      children.add(const SizedBox(height: 22));
    }
    String? day;
    var buf = <ActivityItem>[];
    void flush() {
      if (buf.isEmpty) return;
      children.add(SectionTitle(day!));
      children.add(_group(buf));
      children.add(const SizedBox(height: 22));
      buf = [];
    }

    for (final it in rest) {
      final d = dayLabel(it.startedAt);
      if (d != day) {
        flush();
        day = d;
      }
      buf.add(it);
    }
    flush();
    if (m.activity != null && m.activity!.items.length >= 200) {
      children.add(Padding(padding: const EdgeInsets.only(bottom: 24), child: Text('只显示最近 200 条', style: TextStyle(color: tk.faint))));
    }
    return ListView(padding: const EdgeInsets.only(bottom: 24), children: children);
  }

  Widget _group(List<ActivityItem> items) {
    final tk = context.tk;
    final m = widget.model;
    return Panel(
      padding: EdgeInsets.zero,
      child: Column(children: [
        for (var i = 0; i < items.length; i++) ...[
          if (i > 0) Divider(color: tk.border),
          ActivityRow(item: items[i], selected: items[i].requestId == m.selectedRequestId, onTap: () => m.selectActivity(items[i].requestId)),
        ],
      ]),
    );
  }
}

class _DetailPanel extends StatefulWidget {
  const _DetailPanel({super.key, required this.model, required this.item});
  final AppModel model;
  final ActivityItem item;
  @override
  State<_DetailPanel> createState() => _DetailPanelState();
}

class _DetailPanelState extends State<_DetailPanel> {
  final _out = OutputController();
  bool _cancelling = false;
  String? _notFollowed; // 无法订阅时的说明
  bool _started = false;

  @override
  void initState() {
    super.initState();
    _maybeFollow();
  }

  @override
  void didUpdateWidget(_DetailPanel old) {
    super.didUpdateWidget(old);
    _maybeFollow();
  }

  /// 进行中且知道怎么寻址的 proc.exec 才订阅实时输出。
  void _maybeFollow() {
    final it = widget.item;
    if (_started || !it.running || it.targetRef.isEmpty || it.tool != 'proc.exec') return;
    _started = true;
    unawaited(_out.follow(widget.model.api, it.targetRef, it.requestId, maxTries: 3).then((_) {
      if (mounted && _out.error != null) setState(() => _notFollowed = _out.error);
    }));
  }

  @override
  void dispose() {
    _out.dispose();
    super.dispose();
  }

  Future<void> _cancel() async {
    final it = widget.item;
    final ok = await confirm(
      context,
      title: '取消这条调用？',
      body: '会结束 ${it.targetName} 上 ${it.tool} 启动的进程（含子进程）。记录会变为“已取消”。',
      ok: '取消调用',
      danger: true,
    );
    if (!ok || !mounted) return;
    setState(() => _cancelling = true);
    try {
      await widget.model.api.cancel(it.targetRef, it.requestId);
      await widget.model.refreshActivity();
    } on ApiException catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('取消失败：${e.message}')));
    } finally {
      if (mounted) setState(() => _cancelling = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final it = widget.item;
    final who = it.isAi ? 'AI' : '人';
    final via = switch (it.via) { 'mcp' => 'MCP', 'cli' => '命令行', 'ui' => '客户端', _ => it.via };
    String status = it.running
        ? '进行中'
        : it.ok
            ? '成功'
            : it.cancelled
                ? '已取消'
                : it.status == 'interrupted'
                    ? '已中断（节点退出）'
                    : '失败（${it.status}）';
    final rows = <(String, String)>[
      ('操作者', '$who${via.isEmpty ? '' : ' · $via'}${it.session.isEmpty ? '' : ' · 会话 ${it.session}'}'),
      ('来自设备', it.sourceDevice.isEmpty ? '—' : it.sourceDevice),
      ('目标设备', it.targetName.isEmpty ? '—' : it.targetName),
      ('工具', it.tool),
      ('结果', status),
      ('开始', '${dayLabel(it.startedAt)} ${clock(it.startedAt)}'),
      if (it.endedAt != null) ('耗时', formatDuration(Duration(milliseconds: it.durationMs))),
      ('请求 ID', it.requestId),
    ];
    return Panel(
      padding: const EdgeInsets.all(16),
      child: ListView(children: [
        Row(children: [
          const Expanded(child: Text('详情', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600))),
          IconButton(tooltip: '关闭', onPressed: () => widget.model.selectActivity(null), icon: const Icon(Icons.close, size: 18)),
        ]),
        const SizedBox(height: 6),
        for (final r in rows)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 5),
            child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              SizedBox(width: 72, child: Text(r.$1, style: TextStyle(color: tk.muted, fontSize: 13))),
              Expanded(child: SelectableText(r.$2, style: const TextStyle(fontSize: 13))),
            ]),
          ),
        if (it.running && it.targetRef.isNotEmpty) ...[
          const SizedBox(height: 8),
          FilledButton.icon(
            style: FilledButton.styleFrom(backgroundColor: tk.bad),
            onPressed: _cancelling ? null : _cancel,
            icon: const Icon(Icons.stop, size: 18),
            label: Text(_cancelling ? '正在取消…' : '取消这条调用'),
          ),
        ],
        const SizedBox(height: 10),
        SectionTitle('参数（已脱敏）', trailing: it.args.isEmpty ? null : TextButton(onPressed: () => copyText(context, it.args), child: const Text('复制'))),
        CodeBox(child: SelectableText(it.args.isEmpty ? '（无参数）' : it.args, style: monoStyle(context, size: 12, color: tk.codeText))),
        if (it.running && it.tool == 'proc.exec') ...[
          const SizedBox(height: 16),
          SectionTitle('实时输出', trailing: _out.following ? Text('已连接', style: TextStyle(color: tk.ok, fontSize: 12.5)) : null),
          if (it.targetRef.isEmpty)
            Text('这条调用只有本机发出的记录，无法订阅它的输出。', style: TextStyle(color: tk.muted))
          else ...[
            if (_notFollowed != null) Padding(padding: const EdgeInsets.only(bottom: 6), child: Text(_notFollowed!, style: TextStyle(color: tk.bad, fontSize: 12.5))),
            OutputView(controller: _out, height: 240),
          ],
        ],
        if (!it.running && it.tool == 'proc.exec')
          Padding(
            padding: const EdgeInsets.only(top: 14),
            child: Text('已结束的调用不保存输出内容。', style: TextStyle(color: tk.faint, fontSize: 12.5)),
          ),
      ]),
    );
  }
}
