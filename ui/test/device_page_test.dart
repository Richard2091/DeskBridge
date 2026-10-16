import 'dart:convert';
import 'dart:io';

import 'package:deskbridge_client/app.dart';
import 'package:deskbridge_client/core/api.dart';
import 'package:deskbridge_client/core/app_model.dart';
import 'package:deskbridge_client/core/models.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// 模拟桥接：sys.info、proc.exec、fs.list、fs.read；实时事件订阅一律 404（命令太快没赶上）。
Future<HttpServer> fakeBridge(List<Map<String, dynamic>> calls) async {
  final server = await HttpServer.bind('127.0.0.1', 0);
  server.listen((req) async {
    final path = req.uri.path;
    void json(int status, Object body) {
      req.response.statusCode = status;
      req.response.write(jsonEncode(body));
    }

    if (path.endsWith('/events')) {
      json(404, {'ok': false, 'error': {'code': 'not_found', 'message': '没有正在执行的请求'}});
    } else if (req.method == 'POST' && path.contains('/v1/tools/')) {
      final body = jsonDecode(await utf8.decoder.bind(req).join()) as Map<String, dynamic>;
      final tool = path.split('/').last;
      calls.add({'tool': tool, ...body});
      final args = (body['args'] as Map<String, dynamic>?) ?? {};
      Map<String, dynamic> result;
      switch (tool) {
        case 'sys.info':
          result = {'name': '测试机', 'hostname': 'win-test', 'os': 'windows', 'arch': 'amd64', 'home': 'C:\\Users\\me', 'num_cpu': 8, 'memory_total_bytes': 17179869184};
        case 'proc.exec':
          result = args['command'] == 'fail'
              ? {'exit_code': 3, 'stdout': '', 'stderr': '出错了\n'}
              : {'exit_code': 0, 'stdout': '你好，世界\n', 'stderr': ''};
        case 'fs.list':
          result = {
            'path': args['path'],
            'entries': [
              {'name': 'notes.txt', 'type': 'file', 'size': 12, 'mtime': '2026-10-16T08:00:00Z'},
              {'name': 'Docs', 'type': 'dir', 'size': 0, 'mtime': '2026-10-15T08:00:00Z'},
            ],
            'truncated': false,
          };
        case 'fs.read':
          result = {'path': args['path'], 'content': '第一行\n第二行', 'encoding': 'utf-8', 'eof': true};
        default:
          result = {};
      }
      json(200, {'ok': true, 'request_id': body['request_id'], 'result': result, 'duration_ms': 5});
    } else {
      json(404, {'ok': false, 'error': {'code': 'not_found', 'message': 'x'}});
    }
    await req.response.close();
  });
  return server;
}

Device me() => Device.fromJson({
      'kind': 'tailscale', 'name': 'win-test', 'self': true, 'online': true, 'os': 'windows', 'addrs': ['100.64.0.1'],
      'deskbridge': {'device_id': 'd', 'display_name': '测试机', 'version': 'm2', 'platform': {'os': 'windows'}, 'tools': 9},
    });

void main() {
  testWidgets('终端：执行命令显示输出和退出码；失败命令标红', (tester) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final calls = <Map<String, dynamic>>[];
    late HttpServer server;
    late AppModel model;
    await tester.runAsync(() async {
      server = await fakeBridge(calls);
      model = AppModel(Api('127.0.0.1', server.port, 't'), poll: false);
    });
    addTearDown(() async => tester.runAsync(() => server.close(force: true)));
    model.devices = DeviceList([
      DeviceGroup(kind: 'tailscale', status: ChannelStatus(kind: 'tailscale', state: 'connected'), devices: [me()]),
    ]);
    await tester.pumpWidget(DeskBridgeApp(model: model));
    model.openDeviceNamed(me());
    await tester.pump();

    // 概览读 sys.info。
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 300)));
    await tester.pump();
    expect(find.text('win-test'), findsWidgets);

    await tester.tap(find.text('终端'));
    await tester.pump();
    await tester.enterText(find.byType(TextField).first, 'echo 你好');
    await tester.tap(find.text('执行'));
    await tester.pump();
    expect(find.text('取消'), findsOneWidget); // 运行中显示取消
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 600)));
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.textContaining('你好，世界', findRichText: true), findsOneWidget);
    expect(find.textContaining('退出码 0'), findsOneWidget);
    final call = calls.lastWhere((c) => c['tool'] == 'proc.exec');
    expect(call['initiator'], {'type': 'human', 'via': 'ui'});
    expect((call['args'] as Map)['command'], 'echo 你好');
    expect(call['request_id'], matches(RegExp(r'^[0-9a-f]{32}$')));

    await tester.enterText(find.byType(TextField).first, 'fail');
    await tester.tap(find.text('执行'));
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 600)));
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.textContaining('出错了', findRichText: true), findsOneWidget);
    expect(find.textContaining('退出码 3'), findsOneWidget);
  });

  testWidgets('文件：列出目录、进入文件夹、预览文本', (tester) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final calls = <Map<String, dynamic>>[];
    late HttpServer server;
    late AppModel model;
    await tester.runAsync(() async {
      server = await fakeBridge(calls);
      model = AppModel(Api('127.0.0.1', server.port, 't'), poll: false);
    });
    addTearDown(() async => tester.runAsync(() => server.close(force: true)));
    model.devices = DeviceList([
      DeviceGroup(kind: 'tailscale', status: ChannelStatus(kind: 'tailscale', state: 'connected'), devices: [me()]),
    ]);
    await tester.pumpWidget(DeskBridgeApp(model: model));
    model.openDeviceNamed(me());
    await tester.pump();
    await tester.tap(find.text('文件'));
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 500)));
    await tester.pump();
    // 先读 sys.info 的用户目录，再列出它。
    expect(calls.where((c) => c['tool'] == 'fs.list').first['args'], containsPair('path', 'C:\\Users\\me'));
    expect(find.text('Docs'), findsOneWidget);
    expect(find.text('notes.txt'), findsOneWidget);
    // 文件夹排在文件前面。
    expect(tester.getTopLeft(find.text('Docs')).dy, lessThan(tester.getTopLeft(find.text('notes.txt')).dy));

    await tester.tap(find.text('notes.txt'));
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 300)));
    await tester.pump();
    expect(find.textContaining('第一行', findRichText: true), findsOneWidget);
    final read = calls.lastWhere((c) => c['tool'] == 'fs.read');
    expect((read['args'] as Map)['path'], 'C:\\Users\\me\\notes.txt');

    await tester.tap(find.text('Docs'));
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 300)));
    await tester.pump();
    expect(calls.where((c) => c['tool'] == 'fs.list').last['args'], containsPair('path', 'C:\\Users\\me\\Docs'));
  });
}
