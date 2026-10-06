import 'dart:async';
import 'dart:convert';
import 'dart:io';

/// 把 Go 核心作为子进程启动：exe 与本程序放在同一目录。
class CoreProcess {
  CoreProcess(this.addr);
  final String addr;
  Process? _proc;

  String _findExe() {
    final dir = File(Platform.resolvedExecutable).parent.path;
    final sep = Platform.pathSeparator;
    final p = '$dir${sep}uicore-server.exe';
    if (File(p).existsSync()) return p;
    throw StateError('找不到 $p');
  }

  /// 启动并等待标准输出出现 LISTENING 行（10 秒超时）。
  Future<void> start() async {
    final proc = await Process.start(_findExe(), ['-addr', addr]);
    _proc = proc;
    final ready = Completer<void>();
    proc.stdout
        .transform(utf8.decoder)
        .transform(const LineSplitter())
        .listen((line) {
      if (line.startsWith('LISTENING') && !ready.isCompleted) {
        ready.complete();
      }
    });
    // 必须持续消费 stderr，否则管道写满会让子进程卡住
    proc.stderr.drain<void>();
    proc.exitCode.then((c) {
      if (!ready.isCompleted) {
        ready.completeError(StateError('Go 核心提前退出，退出码 $c'));
      }
    });
    await ready.future.timeout(const Duration(seconds: 10));
  }

  Future<void> stop() async {
    final p = _proc;
    _proc = null;
    if (p == null) return;
    p.kill();
    await p.exitCode.timeout(const Duration(seconds: 3), onTimeout: () => -1);
  }
}