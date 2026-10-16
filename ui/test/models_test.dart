import 'package:deskbridge_client/core/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('设备列表解析：本机、已安装、未安装、离线', () {
    final list = DeviceList.fromJson({
      'groups': [
        {
          'kind': 'tailscale',
          'status': {'kind': 'tailscale', 'state': 'connected', 'account': 'user@example.com'},
          'devices': [
            {
              'kind': 'tailscale', 'name': 'win-pc', 'self': true, 'online': true, 'os': 'windows', 'addrs': ['100.64.0.1'],
              'deskbridge': {'device_id': 'd1', 'display_name': '我的电脑', 'version': 'm2', 'platform': {'os': 'windows', 'arch': 'amd64'}, 'tools': 9},
            },
            {'kind': 'tailscale', 'name': 'srv', 'online': true, 'os': 'linux', 'path': 'relay', 'relay': 'hkg', 'latency_ms': 231.4,
              'deskbridge': {'device_id': 'd2', 'display_name': '', 'version': 'm2', 'platform': {'os': 'linux', 'arch': 'amd64'}, 'tools': 9}},
            {'kind': 'tailscale', 'name': 'phone', 'online': true, 'os': 'android', 'client': 'tailscale'},
            {'kind': 'tailscale', 'name': 'old', 'online': false},
          ],
        }
      ],
    });
    final devs = list.groups.single.devices;
    expect(list.groups.single.status.connected, isTrue);
    expect(devs[0].displayName, '我的电脑');
    expect(devs[0].target, 'self');
    expect(devs[1].displayName, 'srv'); // 设备名称为空时用内网短名
    expect(devs[1].target, 'tailscale:srv');
    expect(devs[1].pathText, '中继（hkg）');
    expect(devs[1].latencyText, '231 毫秒');
    expect(devs[2].hasDeskBridge, isFalse);
    expect(devs[3].online, isFalse);
  });

  test('操作记录解析', () {
    final r = ActivityResult.fromJson({
      'items': [
        {'request_id': 'a', 'role': 'target', 'operator_type': 'ai', 'via': 'mcp', 'tool': 'proc.exec', 'args': '{"command":"dir"}',
          'status': 'running', 'started_at': 1000, 'target_name': '台式', 'target_ref': 'tailscale:desk', 'source_device': 'srv'},
        {'request_id': 'b', 'role': 'source', 'operator_type': 'human', 'tool': 'fs.read', 'status': 'ok', 'started_at': 900, 'ended_at': 950, 'duration_ms': 50},
      ],
      'errors': {'旧设备': '对端不支持这个接口'},
    });
    expect(r.items.length, 2);
    expect(r.items[0].running, isTrue);
    expect(r.items[0].isAi, isTrue);
    expect(r.items[0].endedAt, isNull);
    expect(r.items[1].ok, isTrue);
    expect(r.items[1].endedAt, isNotNull);
    expect(r.errors['旧设备'], contains('不支持'));
  });

  test('缺字段不抛异常', () {
    expect(ActivityItem.fromJson({}).requestId, '');
    expect(Device.fromJson({}).displayName, '');
    expect(LocalStatus.fromJson({}).channels, isEmpty);
  });
}
