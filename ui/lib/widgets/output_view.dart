// 命令输出显示：stdout 正常色，stderr 红色；新内容到来时自动滚动到底部。

import 'package:flutter/material.dart';

import '../core/output.dart';
import '../theme.dart';
import 'common.dart';

class OutputView extends StatefulWidget {
  const OutputView({super.key, required this.controller, this.height = 320, this.placeholder = '（还没有输出）'});
  final OutputController controller;
  final double height;
  final String placeholder;

  @override
  State<OutputView> createState() => _OutputViewState();
}

class _OutputViewState extends State<OutputView> {
  final _scroll = ScrollController();

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_onChange);
  }

  @override
  void didUpdateWidget(OutputView old) {
    super.didUpdateWidget(old);
    if (old.controller != widget.controller) {
      old.controller.removeListener(_onChange);
      widget.controller.addListener(_onChange);
    }
  }

  @override
  void dispose() {
    widget.controller.removeListener(_onChange);
    _scroll.dispose();
    super.dispose();
  }

  void _onChange() {
    if (!mounted) return;
    setState(() {});
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) _scroll.jumpTo(_scroll.position.maxScrollExtent);
    });
  }

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    final c = widget.controller;
    final base = monoStyle(context, color: tk.codeText);
    return CodeBox(
      height: widget.height,
      child: Scrollbar(
        controller: _scroll,
        child: SingleChildScrollView(
          controller: _scroll,
          child: SizedBox(
            width: double.infinity,
            child: SelectableText.rich(
              TextSpan(style: base, children: [
                if (c.droppedBytes > 0)
                  TextSpan(text: '（较早的 ${c.droppedBytes} 字节输出已省略）\n', style: base.copyWith(color: tk.warn)),
                if (!c.hasOutput && c.droppedBytes == 0) TextSpan(text: widget.placeholder, style: base.copyWith(color: tk.faint)),
                for (final ch in c.chunks)
                  TextSpan(text: ch.text, style: ch.stream == 'stderr' ? base.copyWith(color: const Color(0xFFF87171)) : base),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
