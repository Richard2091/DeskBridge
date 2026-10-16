import 'dart:convert';
import 'dart:io';

import 'package:deskbridge_client/core/api.dart';
import 'package:deskbridge_client/core/models.dart';
import 'package:deskbridge_client/core/output.dart';
import 'package:flutter_test/flutter_test.dart';

/// 模拟桥接进程：校验令牌，返回固定数据，并在 WebSocket 上推送事件。
Future<HttpServer> fakeBridge(List<String> log) async {
  final server = await HttpServer.bind('127.0.0.1', 0);
  server.listen((req) async {
    if (req.headers.value('authorization') != 'Bearer tok') {
      req.response.statusCode = 401;
      req.response.write(jsonEncode({'ok': false, 'error': {'code': 'unauthorized', 'message': '缺少或错误的令牌'}}));
      await req.response.close();
      return;
    }
    final path = req.uri.path;
    log.add('${req.method} $path');
    if (path.endsWith('/events')) {
      if (path.contains('gone')) {
        req.response.statusCode = 404;
        req.response.write(jsonEncode({'ok': false, 'error': {'code': 'not_found', 'message': '没有正在执行的请求'}}));
        await req.response.close();
        return;
      }
      final ws = await WebSocketTransformer.upgrade(req);
      ws.add(jsonEncode({'type': 'start', 'tool': 'proc.exec', 'dropped_bytes': 5}));
      ws.add(jsonEncode({'type': 'output', 'stream': 'stdout', 'data': '你好\n'}));
      ws.add(jsonEncode({'type': 'output', 'stream': 'stdout', 'data': '世界\n'}));
      ws.add(jsonEncode({'type': 'output', 'stream': 'stderr', 'data': 'oops\n'}));
      ws.add(jsonEncode({'type': 'end', 'code': 'ok', 'duration_ms': 12}));
      await ws.close();
      return;
    }
    if (req.method == 'POST' && path.contains('/v1/tools/')) {
      final body = jsonDecode(await utf8.decoder.bind(req).join()) as Map<String, dynamic>;
      log.add('body ${jsonEncode(body)}');
      if (path.contains('no.such')) {
        req.response.statusCode = 404;
        req.response.write(jsonEncode({'ok': false, 'error': {'code': 'unknown_tool', 'message': '没有这个工具'}}));
      } else {
        req.response.write(jsonEncode({'ok': true, 'request_id': body['request_id'], 'result': {'exit_code': 0, 'stdout': 'hi'}, 'duration_ms': 7}));
      }
      await req.response.close();
      return;
    }
    if (path == '/local/devices') {
      req.response.write(jsonEncode({'groups': []}));
    } else if (path == '/api/activity') {
      req.response.write(jsonEncode({'items': [{'request_id': 'x', 'operator_type': 'ai', 'tool': 't', 'status': 'ok', 'started_at': 1}]}));
    } else {
      req.response.statusCode = 404;
    }
    await req.response.close();
  });
  return server;
}

void main() {
  late HttpServer server;
  late Api api;
  final log = <String>[];

  setUp(() async {
    log.clear();
    server = await fakeBridge(log);
    api = Api('127.0.0.1', server.port, 'tok');
  });
  tearDown(() async {
    api.close();
    await server.close(force: true);
  });

  test('错误的令牌得到 unauthorized', () async {
    final bad = Api('127.0.0.1', server.port, 'wrong');
    await expectLater(bad.devices(), throwsA(isA<ApiException>().having((e) => e.code, 'code', 'unauthorized')));
    bad.close();
  });

  test('读取设备和记录', () async {
    expect((await api.devices()).groups, isEmpty);
    final a = await api.activity(operatorType: 'ai');
    expect(a.items.single.requestId, 'x');
    expect(log, contains('GET /api/activity'));
  });

  test('调用工具带 request_id 和“人、经界面”发起者', () async {
    final r = await api.callTool('tailscale:srv', 'proc.exec', {'command': 'echo hi'}, requestId: 'rid-1');
    expect(r.requestId, 'rid-1');
    expect(r.result['stdout'], 'hi');
    final body = jsonDecode(log.firstWhere((l) => l.startsWith('body ')).substring(5)) as Map<String, dynamic>;
    expect(body['request_id'], 'rid-1');
    expect(body['initiator'], {'type': 'human', 'via': 'ui'});
    expect(log, contains('POST /local/devices/tailscale%3Asrv/v1/tools/proc.exec'));
  });

  test('工具错误转成 ApiException', () async {
    await expectLater(api.callTool('self', 'no.such', null), throwsA(isA<ApiException>().having((e) => e.code, 'code', 'unknown_tool')));
  });

  test('订阅实时事件并写入输出控制器', () async {
    final out = OutputController();
    await out.follow(api, 'self', 'abc');
    await Future<void>.delayed(const Duration(milliseconds: 200)); // 等合并通知
    expect(out.droppedBytes, 5);
    expect(out.chunks.length, 2); // 两段 stdout 合并成一段，再加一段 stderr
    expect(out.chunks[0].text, '你好\n世界\n');
    expect(out.chunks[1].stream, 'stderr');
    expect(out.ended, isTrue);
    expect(out.endCode, 'ok');
    out.dispose();
  });

  test('订阅已结束的调用得到 not_found，重试后放弃且不报错', () async {
    await expectLater(api.events('self', 'gone'), throwsA(isA<ApiException>().having((e) => e.code, 'code', 'not_found')));
    final out = OutputController();
    await out.follow(api, 'self', 'gone', maxTries: 3, retryEvery: const Duration(milliseconds: 10));
    expect(out.error, isNull);
    expect(out.hasOutput, isFalse);
    expect(log.where((l) => l.endsWith('/events')).length, greaterThanOrEqualTo(4)); // 1 次直接订阅 + 3 次重试
    out.dispose();
  });

  test('newRequestId 是 32 位十六进制且不重复', () {
    final a = Api.newRequestId(), b = Api.newRequestId();
    expect(a, matches(RegExp(r'^[0-9a-f]{32}$')));
    expect(a, isNot(b));
  });
}
