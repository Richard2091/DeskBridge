// 各页面共用的小控件。

import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme.dart';

/// 白色卡片。
class Panel extends StatelessWidget {
  const Panel({super.key, required this.child, this.padding = const EdgeInsets.all(20), this.color});
  final Widget child;
  final EdgeInsetsGeometry padding;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Container(
      padding: padding,
      decoration: BoxDecoration(
        color: color ?? tk.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: tk.border),
      ),
      child: child,
    );
  }
}

/// 状态点。颜色之外总是配文字（无障碍：不能只靠颜色）。
class StatusDot extends StatelessWidget {
  const StatusDot(this.color, {super.key, this.size = 9});
  final Color color;
  final double size;
  @override
  Widget build(BuildContext context) =>
      Container(width: size, height: size, decoration: BoxDecoration(color: color, shape: BoxShape.circle));
}

/// 带浅底色的小标签。
class Pill extends StatelessWidget {
  const Pill(this.text, {super.key, required this.fg, required this.bg, this.icon});
  final String text;
  final Color fg, bg;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(999)),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        if (icon != null) ...[Icon(icon, size: 12, color: fg), const SizedBox(width: 4)],
        Text(text, style: TextStyle(fontSize: 12, color: fg, fontWeight: FontWeight.w500)),
      ]),
    );
  }
}

class SectionTitle extends StatelessWidget {
  const SectionTitle(this.text, {super.key, this.trailing});
  final String text;
  final Widget? trailing;
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Row(children: [
        Text(text, style: TextStyle(fontSize: 13, color: context.tk.muted, fontWeight: FontWeight.w600)),
        const Spacer(),
        if (trailing != null) trailing!,
      ]),
    );
  }
}

/// 页面标题行。
class PageTitle extends StatelessWidget {
  const PageTitle(this.title, {super.key, this.actions = const [], this.leading});
  final String title;
  final List<Widget> actions;
  final Widget? leading;
  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 20),
      child: Row(children: [
        if (leading != null) ...[leading!, const SizedBox(width: 8)],
        Expanded(child: Text(title, style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w600))),
        ...actions,
      ]),
    );
  }
}

/// 空状态和错误状态用的居中提示。
class Notice extends StatelessWidget {
  const Notice({super.key, required this.icon, required this.title, this.detail, this.action, this.color});
  final IconData icon;
  final String title;
  final String? detail;
  final Widget? action;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 440),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Icon(icon, size: 36, color: color ?? tk.faint),
            const SizedBox(height: 12),
            Text(title, textAlign: TextAlign.center, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
            if (detail != null) ...[
              const SizedBox(height: 6),
              Text(detail!, textAlign: TextAlign.center, style: TextStyle(color: tk.muted, height: 1.5)),
            ],
            if (action != null) ...[const SizedBox(height: 16), action!],
          ]),
        ),
      ),
    );
  }
}

/// 横幅提示（出错、设备离线等）。
class Banner2 extends StatelessWidget {
  const Banner2(this.text, {super.key, required this.fg, required this.bg, this.icon = Icons.info_outline, this.action});
  final String text;
  final Color fg, bg;
  final IconData icon;
  final Widget? action;
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(10)),
      child: Row(children: [
        Icon(icon, size: 18, color: fg),
        const SizedBox(width: 10),
        Expanded(child: Text(text, style: TextStyle(color: fg, height: 1.4))),
        if (action != null) action!,
      ]),
    );
  }
}

/// 简单的标签页头（自绘，便于保持各页状态用 IndexedStack）。
class TabStrip extends StatelessWidget {
  const TabStrip({super.key, required this.labels, required this.index, required this.onChanged, this.trailing});
  final List<String> labels;
  final int index;
  final ValueChanged<int> onChanged;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Container(
      decoration: BoxDecoration(border: Border(bottom: BorderSide(color: tk.border))),
      child: Row(children: [
        for (var i = 0; i < labels.length; i++)
          Semantics(
            button: true,
            selected: i == index,
            label: labels[i],
            child: InkWell(
              onTap: () => onChanged(i),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 11),
                decoration: BoxDecoration(
                  border: Border(bottom: BorderSide(color: i == index ? tk.accent : Colors.transparent, width: 2)),
                ),
                child: Text(
                  labels[i],
                  style: TextStyle(
                    fontWeight: i == index ? FontWeight.w600 : FontWeight.w400,
                    color: i == index ? tk.text : tk.muted,
                  ),
                ),
              ),
            ),
          ),
        const Spacer(),
        if (trailing != null) trailing!,
      ]),
    );
  }
}

/// 等宽文本框（命令输出、参数摘要等）。
class CodeBox extends StatelessWidget {
  const CodeBox({super.key, required this.child, this.height, this.padding = const EdgeInsets.all(12)});
  final Widget child;
  final double? height;
  final EdgeInsets padding;
  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Container(
      height: height,
      width: double.infinity,
      padding: padding,
      decoration: BoxDecoration(color: tk.codeBg, borderRadius: BorderRadius.circular(10)),
      child: child,
    );
  }
}

TextStyle monoStyle(BuildContext context, {double size = 13, Color? color}) => TextStyle(
      fontFamily: monoFamily,
      fontFamilyFallback: monoFallback,
      fontSize: size,
      height: 1.45,
      color: color ?? context.tk.text,
    );

/// 复制文本到剪贴板并提示。
Future<void> copyText(BuildContext context, String text, {String done = '已复制'}) async {
  await Clipboard.setData(ClipboardData(text: text));
  if (context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(done), duration: const Duration(seconds: 2)));
  }
}

/// 在系统浏览器中打开链接。
Future<void> openUrl(String url) async {
  try {
    if (Platform.isWindows) {
      await Process.run('rundll32', ['url.dll,FileProtocolHandler', url]);
    } else if (Platform.isMacOS) {
      await Process.run('open', [url]);
    } else {
      await Process.run('xdg-open', [url]);
    }
  } catch (_) {}
}

String formatBytes(int n) {
  if (n < 1024) return '$n 字节';
  if (n < 1024 * 1024) return '${(n / 1024).toStringAsFixed(1)} KB';
  if (n < 1024 * 1024 * 1024) return '${(n / 1024 / 1024).toStringAsFixed(1)} MB';
  return '${(n / 1024 / 1024 / 1024).toStringAsFixed(1)} GB';
}

String formatDuration(Duration d) {
  if (d.inMilliseconds < 1000) return '${d.inMilliseconds} 毫秒';
  if (d.inSeconds < 60) return '${(d.inMilliseconds / 1000).toStringAsFixed(1)} 秒';
  if (d.inMinutes < 60) return '${d.inMinutes} 分 ${d.inSeconds % 60} 秒';
  return '${d.inHours} 小时 ${d.inMinutes % 60} 分';
}

String two(int n) => n.toString().padLeft(2, '0');
String clock(DateTime t) => '${two(t.hour)}:${two(t.minute)}:${two(t.second)}';
String dayLabel(DateTime t) {
  final now = DateTime.now();
  final d = DateTime(t.year, t.month, t.day);
  final today = DateTime(now.year, now.month, now.day);
  final diff = today.difference(d).inDays;
  if (diff == 0) return '今天';
  if (diff == 1) return '昨天';
  return '${t.year}-${two(t.month)}-${two(t.day)}';
}

/// 带“取消”的确认对话框。
Future<bool> confirm(BuildContext context, {required String title, required String body, String ok = '确定', bool danger = false}) async {
  final r = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text(title),
      content: Text(body),
      actions: [
        TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
        FilledButton(
          style: danger ? FilledButton.styleFrom(backgroundColor: ctx.tk.bad) : null,
          onPressed: () => Navigator.pop(ctx, true),
          child: Text(ok),
        ),
      ],
    ),
  );
  return r ?? false;
}
