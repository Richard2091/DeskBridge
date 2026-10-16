import 'package:deskbridge_client/app.dart';
import 'package:deskbridge_client/core/api.dart';
import 'package:deskbridge_client/core/app_model.dart';
import 'package:deskbridge_client/core/models.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

Device dev(String name, {bool self = false, bool online = true, bool db = true, String os = 'windows'}) => Device.fromJson({
      'kind': 'tailscale',
      'name': name,
      'self': self,
      'online': online,
      'os': os,
      'path': 'relay',
      'relay': 'hkg',
      'latency_ms': 230,
      'addrs': ['100.64.0.1'],
      if (db) 'deskbridge': {'device_id': name, 'display_name': '设备$name', 'version': 'm2', 'platform': {'os': os}, 'tools': 9},
    });

AppModel fakeModel() {
  final m = AppModel(Api('127.0.0.1', 1, 't'), poll: false);
  m.devices = DeviceList([
    DeviceGroup(
      kind: 'tailscale',
      status: ChannelStatus(kind: 'tailscale', state: 'connected', account: 'user@example.com'),
      devices: [dev('me', self: true), dev('srv', os: 'linux'), dev('off', online: false), dev('phone', db: false, os: 'android')],
    ),
  ]);
  m.status = LocalStatus(deviceId: 'abcdef', name: '我的电脑', version: 'm2', channels: [ChannelStatus(kind: 'tailscale', state: 'connected', account: 'user@example.com')]);
  m.activity = ActivityResult([
    ActivityItem.fromJson({'request_id': 'r1', 'role': 'target', 'operator_type': 'ai', 'via': 'mcp', 'tool': 'fs.read', 'args': '{"path":"D:\\\\a.txt"}',
      'status': 'running', 'started_at': DateTime.now().millisecondsSinceEpoch, 'target_name': '设备srv', 'target_ref': 'tailscale:srv', 'source_device': 'laptop'}),
    ActivityItem.fromJson({'request_id': 'r2', 'role': 'target', 'operator_type': 'human', 'via': 'ui', 'tool': 'proc.exec', 'args': '{"command":"dir"}',
      'status': 'ok', 'started_at': DateTime.now().millisecondsSinceEpoch - 5000, 'ended_at': DateTime.now().millisecondsSinceEpoch - 4000, 'duration_ms': 1000, 'target_name': '设备me', 'target_ref': 'self'}),
  ], {});
  return m;
}

Future<void> pumpApp(WidgetTester tester, AppModel m) async {
  tester.view.physicalSize = const Size(1280, 800);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(DeskBridgeApp(model: m));
  await tester.pump();
}

void main() {
  testWidgets('设备页：本机和在线设备一行一台，其余折叠', (tester) async {
    final m = fakeModel();
    await pumpApp(tester, m);
    expect(find.text('设备me'), findsOneWidget);
    expect(find.text('设备srv'), findsOneWidget);
    expect(find.textContaining('还有 2 台离线或未安装 DeskBridge 的设备'), findsOneWidget);
    expect(find.text('设备off'), findsNothing); // 折叠中
    await tester.tap(find.textContaining('还有 2 台'));
    await tester.pump();
    expect(find.text('设备off'), findsOneWidget);
    // 侧栏的进行中数量。
    expect(find.text('1'), findsWidgets);
  });

  testWidgets('点设备进入设备页，有概览、终端、文件标签，可返回', (tester) async {
    final m = fakeModel();
    await pumpApp(tester, m);
    m.openDeviceNamed(m.devices!.groups.first.devices[1]);
    await tester.pump();
    expect(find.text('终端'), findsOneWidget);
    expect(find.text('文件'), findsOneWidget);
    expect(find.text('远程桌面'), findsOneWidget);
    // AI 正在这台设备上执行的提示条。
    expect(find.textContaining('AI 正在这台设备上执行 fs.read'), findsOneWidget);
    await tester.tap(find.byTooltip('返回设备列表'));
    await tester.pump();
    expect(find.text('设备'), findsWidgets);
    expect(m.openDeviceTarget, isNull);
  });

  testWidgets('操作记录页：进行中在最前，点开详情有取消按钮', (tester) async {
    final m = fakeModel();
    await pumpApp(tester, m);
    m.goTo(AppPage.activity);
    await tester.pump();
    expect(find.text('进行中'), findsWidgets);
    expect(find.text('fs.read'), findsOneWidget);
    expect(find.text('proc.exec'), findsOneWidget);
    await tester.tap(find.text('fs.read'));
    await tester.pump();
    expect(find.text('详情'), findsOneWidget);
    expect(find.text('取消这条调用'), findsOneWidget);
    // 筛选“人”后只剩人发起的。
    await tester.tap(find.descendant(of: find.byType(SegmentedButton<String>), matching: find.text('人')));
    await tester.pump();
    expect(find.text('fs.read'), findsNothing);
    expect(find.text('proc.exec'), findsOneWidget);
  });

  testWidgets('设置页：本机信息和主题切换', (tester) async {
    final m = fakeModel();
    await pumpApp(tester, m);
    m.goTo(AppPage.settings);
    await tester.pump();
    expect(find.text('我的电脑'), findsOneWidget);
    await tester.tap(find.text('外观'));
    await tester.pump();
    await tester.tap(find.text('深色'));
    await tester.pump();
    expect(m.themeMode, ThemeMode.dark);
  });

  testWidgets('读不到设备列表时显示原因和重试', (tester) async {
    final m = AppModel(Api('127.0.0.1', 1, 't'), poll: false);
    m.devicesError = '本机节点未运行。';
    await pumpApp(tester, m);
    expect(find.text('无法读取设备列表'), findsOneWidget);
    expect(find.text('本机节点未运行。'), findsOneWidget);
    expect(find.text('重试'), findsOneWidget);
  });

  testWidgets('Tailscale 未登录时显示登录组件', (tester) async {
    final m = AppModel(Api('127.0.0.1', 1, 't'), poll: false);
    m.devices = DeviceList([
      DeviceGroup(kind: 'tailscale', status: ChannelStatus(kind: 'tailscale', state: 'needs_login', loginUrl: 'https://login.example.com/a'), devices: []),
    ]);
    await pumpApp(tester, m);
    expect(find.text('登录 Tailscale'), findsOneWidget);
    expect(find.text('登录'), findsOneWidget);
    expect(find.text('https://login.example.com/a'), findsOneWidget);
  });
}
