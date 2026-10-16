// 一条调用的实时输出：把 WebSocket 事件攒成可显示的片段。

import 'dart:async';

import 'package:flutter/foundation.dart';

import 'api.dart';
import 'models.dart';

class OutputChunk {
  OutputChunk(this.stream, this.text);
  final String stream; // stdout / stderr
  final String text;
}

class OutputController extends ChangeNotifier {
  final List<OutputChunk> chunks = [];
  int droppedBytes = 0; // 更早被节点丢弃的输出
  bool ended = false;
  String endCode = '';
  bool following = false; // 正在订阅
  String? error;
  bool _disposed = false;
  Timer? _pending;

  bool get hasOutput => chunks.isNotEmpty;

  void clear() {
    chunks.clear();
    droppedBytes = 0;
    ended = false;
    endCode = '';
    error = null;
    _poke();
  }

  void apply(CallEvent e) {
    switch (e.type) {
      case 'start':
        droppedBytes = e.droppedBytes;
      case 'output':
        // 相邻同一路的片段合并，减少控件数量。
        if (chunks.isNotEmpty && chunks.last.stream == e.stream) {
          chunks[chunks.length - 1] = OutputChunk(e.stream, chunks.last.text + e.data);
        } else {
          chunks.add(OutputChunk(e.stream, e.data));
        }
      case 'end':
        ended = true;
        endCode = e.code;
    }
    _poke();
  }

  /// 合并短时间内的多次更新，避免高频输出时反复重建界面。
  void _poke() {
    if (_disposed || _pending != null) return;
    _pending = Timer(const Duration(milliseconds: 60), () {
      _pending = null;
      if (!_disposed) notifyListeners();
    });
  }

  /// 订阅 [requestId] 的实时事件并写入本控制器。调用刚发出时可能还没登记，
  /// not_found 时每 [retryEvery] 重试一次，直到 [giveUp] 返回 true 或超过 [maxTries]。
  /// 返回时订阅已结束（调用结束、断开或放弃）。
  Future<void> follow(
    Api api,
    String target,
    String requestId, {
    int maxTries = 1,
    Duration retryEvery = const Duration(milliseconds: 200),
    bool Function()? giveUp,
  }) async {
    following = true;
    error = null;
    _poke();
    try {
      for (var i = 0; i < maxTries; i++) {
        if (_disposed || (giveUp?.call() ?? false)) return;
        try {
          final stream = await api.events(target, requestId);
          await for (final e in stream) {
            if (_disposed) return;
            apply(e);
          }
          return;
        } on ApiException catch (e) {
          if (e.code != 'not_found' || i == maxTries - 1) {
            error = e.code == 'not_found' ? null : e.message;
            return;
          }
          await Future<void>.delayed(retryEvery);
        }
      }
    } catch (e) {
      error = '$e';
    } finally {
      following = false;
      _poke();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _pending?.cancel();
    super.dispose();
  }
}
