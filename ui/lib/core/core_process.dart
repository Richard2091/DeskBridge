// 启动 Go 后台（deskbridge ui-core）作为子进程，读出它的地址和令牌。

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'api.dart';
import 'models.dart';

class CoreProcess {
  CoreProcess._(this._proc, this.api, this.nodeMode);
  final Process _proc;
  final Api api;

  /// 本机节点的来源：external（已在后台运行，复用）、embedded（随客户端运行）、off（不启动）。
  final String nodeMode;

  /// 找 deskbridge 程序：环境变量 DESKBRIDGE_EXE，其次与本程序同目录，最后在 PATH 里找。
  static Future<String> findExe() async {
    final env = Platform.environment['DESKBRIDGE_EXE'];
    if (env != null && env.isNotEmpty && File(env).existsSync()) return env;
    final dir = File(Platform.resolvedExecutable).parent.path;
    final name = Platform.isWindows ? 'deskbridge.exe' : 'deskbridge';
    final beside = '$dir${Platform.pathSeparator}$name';
    if (File(beside).existsSync()) return beside;
    try {
      final r = await Process.run(Platform.isWindows ? 'where' : 'which', ['deskbridge']);
      if (r.exitCode == 0) {
        final first = (r.stdout as String).split(RegExp(r'\r?\n')).first.trim();
        if (first.isNotEmpty) return first;
      }
    } catch (_) {}
    throw ApiException('core_not_found', '找不到 $name：请放在客户端同一目录，或设置环境变量 DESKBRIDGE_EXE');
  }

  /// 启动后台并等它打印地址（30 秒超时；内置节点最多等 15 秒、超时后收尾最多 3 秒）。
  static Future<CoreProcess> start({String? exe}) async {
    final path = exe ?? await findExe();
    final proc = await Process.start(path, ['ui-core']);
    // 必须持续消费 stderr，否则管道写满会让子进程卡住；保留最后约 4 KB 用于报错。
    final errTail = StringBuffer();
    proc.stderr.transform(const Utf8Decoder(allowMalformed: true)).listen((t) {
      errTail.write(t);
      if (errTail.length > 4096) {
        final keep = errTail.toString().substring(errTail.length - 4096);
        errTail
          ..clear()
          ..write(keep);
      }
    });
    final first = proc.stdout.transform(utf8.decoder).transform(const LineSplitter()).first;
    try {
      final line = await first.timeout(const Duration(seconds: 30));
      final j = jsonDecode(line) as Map<String, dynamic>;
      final addr = '${j['addr']}';
      final i = addr.lastIndexOf(':');
      return CoreProcess._(proc, Api(addr.substring(0, i), int.parse(addr.substring(i + 1)), '${j['token']}'), '${j['node'] ?? ''}');
    } catch (e) {
      proc.kill();
      // 给 stderr 一点时间读完最后的输出。
      await Future<void>.delayed(const Duration(milliseconds: 100));
      final detail = errTail.toString().trim();
      throw ApiException('core_start_failed', detail.isNotEmpty ? detail : '后台进程启动失败：${e is StateError ? '进程已退出' : e}');
    }
  }

  /// 先关闭到后台的连接和它的标准输入（后台据此退出，内置节点随之收尾），
  /// 最多等 12 秒，仍没退出才强行结束。
  Future<void> stop() async {
    api.close();
    try {
      await _proc.stdin.close();
    } catch (_) {
      // 进程已经退出，管道已断。
    }
    try {
      await _proc.exitCode.timeout(const Duration(seconds: 12));
    } on TimeoutException {
      _proc.kill();
    }
  }
}
