// 启动 Go 后台（deskbridge ui-core）作为子进程，读出它的地址和令牌。

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'api.dart';
import 'models.dart';

class CoreProcess {
  CoreProcess._(this._proc, this.api);
  final Process _proc;
  final Api api;

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

  /// 启动后台并等它打印地址（10 秒超时）。
  static Future<CoreProcess> start({String? exe}) async {
    final path = exe ?? await findExe();
    final proc = await Process.start(path, ['ui-core']);
    // 必须持续消费 stderr，否则管道写满会让子进程卡住。
    proc.stderr.drain<void>();
    final first = proc.stdout.transform(utf8.decoder).transform(const LineSplitter()).first;
    try {
      final line = await first.timeout(const Duration(seconds: 10));
      final j = jsonDecode(line) as Map<String, dynamic>;
      final addr = '${j['addr']}';
      final i = addr.lastIndexOf(':');
      return CoreProcess._(proc, Api(addr.substring(0, i), int.parse(addr.substring(i + 1)), '${j['token']}'));
    } catch (e) {
      proc.kill();
      throw ApiException('core_start_failed', '后台进程启动失败：$e');
    }
  }

  Future<void> stop() async {
    api.close();
    _proc.kill(); // 后台也会在标准输入关闭时自己退出
  }
}
