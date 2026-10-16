// 本机接口和桥接返回的数据结构。字段名对应 Go 端的 JSON 键
// （internal/client、internal/protocol、internal/mesh）。

class ApiException implements Exception {
  ApiException(this.code, this.message);
  final String code;
  final String message;
  @override
  String toString() => '$message（$code）';
}

String _s(Object? v) => v is String ? v : '';
bool _b(Object? v) => v is bool ? v : false;
int _i(Object? v) => v is num ? v.toInt() : 0;
double _d(Object? v) => v is num ? v.toDouble() : 0;
Map<String, dynamic> _m(Object? v) => v is Map<String, dynamic> ? v : const {};
List<dynamic> _l(Object? v) => v is List ? v : const [];

/// 连接方式的状态（mesh.Status）。
class ChannelStatus {
  ChannelStatus({
    required this.kind,
    required this.state,
    this.loginUrl = '',
    this.reason = '',
    this.account = '',
  });
  factory ChannelStatus.fromJson(Map<String, dynamic> j) => ChannelStatus(
        kind: _s(j['kind']),
        state: _s(j['state']),
        loginUrl: _s(j['login_url']),
        reason: _s(j['reason']),
        account: _s(j['account']),
      );
  final String kind;
  final String state; // starting / needs_login / connected / disconnected / stopped
  final String loginUrl;
  final String reason;
  final String account;

  bool get connected => state == 'connected';
  bool get needsLogin => state == 'needs_login';
}

/// 本机节点状态（GET /local/status）。
class LocalStatus {
  LocalStatus({required this.deviceId, required this.name, required this.version, required this.channels});
  factory LocalStatus.fromJson(Map<String, dynamic> j) => LocalStatus(
        deviceId: _s(j['device_id']),
        name: _s(j['name']),
        version: _s(j['version']),
        channels: _l(j['channels']).map((e) => ChannelStatus.fromJson(_m(e))).toList(),
      );
  final String deviceId;
  final String name;
  final String version;
  final List<ChannelStatus> channels;
}

/// 设备列表里的一台设备（client.Device）。
class Device {
  Device({
    required this.kind,
    required this.name,
    required this.hostName,
    required this.os,
    required this.online,
    required this.path,
    required this.relay,
    required this.latencyMs,
    required this.self,
    required this.addrs,
    this.deskbridgeName,
    this.deskbridgeVersion,
    this.deskbridgeTools = 0,
    this.deskbridgeId,
    this.client = '',
    this.probeError = '',
  });
  factory Device.fromJson(Map<String, dynamic> j) {
    final db = j['deskbridge'];
    final dbm = db is Map<String, dynamic> ? db : null;
    final plat = _m(dbm?['platform']);
    return Device(
      kind: _s(j['kind']),
      name: _s(j['name']),
      hostName: _s(j['host_name']),
      os: _s(j['os']).isNotEmpty ? _s(j['os']) : _s(plat['os']),
      online: _b(j['online']),
      path: _s(j['path']),
      relay: _s(j['relay']),
      latencyMs: _d(j['latency_ms']),
      self: _b(j['self']),
      addrs: _l(j['addrs']).whereType<String>().toList(),
      deskbridgeName: dbm == null ? null : _s(dbm['display_name']),
      deskbridgeVersion: dbm == null ? null : _s(dbm['version']),
      deskbridgeTools: dbm == null ? 0 : _i(dbm['tools']),
      deskbridgeId: dbm == null ? null : _s(dbm['device_id']),
      client: _s(j['client']),
      probeError: _s(j['probe_error']),
    );
  }
  final String kind;
  final String name; // 内网短名
  final String hostName;
  final String os;
  final bool online;
  final String path; // lan_direct / direct / relay / unknown
  final String relay;
  final double latencyMs;
  final bool self;
  final List<String> addrs;
  final String? deskbridgeName;
  final String? deskbridgeVersion;
  final int deskbridgeTools;
  final String? deskbridgeId;
  final String client;
  final String probeError;

  bool get hasDeskBridge => deskbridgeName != null;
  String get displayName => (deskbridgeName?.isNotEmpty ?? false) ? deskbridgeName! : name;

  /// 经本机接口寻址这台设备的写法：本机用 self，其他用“连接方式:短名”。
  String get target => self ? 'self' : '$kind:$name';

  String get pathText {
    switch (path) {
      case 'lan_direct':
        return '局域网直连';
      case 'direct':
        return '跨网直连';
      case 'relay':
        return relay.isEmpty ? '中继' : '中继（$relay）';
      default:
        return '路径未知';
    }
  }

  String get latencyText => latencyMs > 0 ? '${latencyMs.round()} 毫秒' : '';
}

class DeviceGroup {
  DeviceGroup({required this.kind, required this.status, required this.devices, this.error = ''});
  factory DeviceGroup.fromJson(Map<String, dynamic> j) => DeviceGroup(
        kind: _s(j['kind']),
        status: ChannelStatus.fromJson(_m(j['status'])),
        devices: _l(j['devices']).map((e) => Device.fromJson(_m(e))).toList(),
        error: _s(j['error']),
      );
  final String kind;
  final ChannelStatus status;
  final List<Device> devices;
  final String error;
}

class DeviceList {
  DeviceList(this.groups);
  factory DeviceList.fromJson(Map<String, dynamic> j) =>
      DeviceList(_l(j['groups']).map((e) => DeviceGroup.fromJson(_m(e))).toList());
  final List<DeviceGroup> groups;
}

/// 一条合并后的操作记录（client.ActivityItem）。
class ActivityItem {
  ActivityItem({
    required this.requestId,
    required this.role,
    required this.operatorType,
    required this.via,
    required this.session,
    required this.tool,
    required this.args,
    required this.status,
    required this.startedAt,
    required this.endedAt,
    required this.durationMs,
    required this.targetName,
    required this.targetRef,
    required this.sourceDevice,
  });
  factory ActivityItem.fromJson(Map<String, dynamic> j) => ActivityItem(
        requestId: _s(j['request_id']),
        role: _s(j['role']),
        operatorType: _s(j['operator_type']),
        via: _s(j['via']),
        session: _s(j['session']),
        tool: _s(j['tool']),
        args: _s(j['args']),
        status: _s(j['status']),
        startedAt: DateTime.fromMillisecondsSinceEpoch(_i(j['started_at'])),
        endedAt: _i(j['ended_at']) == 0 ? null : DateTime.fromMillisecondsSinceEpoch(_i(j['ended_at'])),
        durationMs: _i(j['duration_ms']),
        targetName: _s(j['target_name']),
        targetRef: _s(j['target_ref']),
        sourceDevice: _s(j['source_device']),
      );
  final String requestId;
  final String role;
  final String operatorType; // ai / human
  final String via; // cli / mcp / ui
  final String session;
  final String tool;
  final String args; // 已脱敏的摘要
  final String status; // running / ok / 错误码 / interrupted
  final DateTime startedAt;
  final DateTime? endedAt;
  final int durationMs;
  final String targetName;
  final String targetRef; // 空表示不知道怎么寻址（只有控制端记录）
  final String sourceDevice;

  bool get running => status == 'running';
  bool get isAi => operatorType == 'ai';
  bool get ok => status == 'ok';
  bool get cancelled => status == 'cancelled';
}

class ActivityResult {
  ActivityResult(this.items, this.errors);
  factory ActivityResult.fromJson(Map<String, dynamic> j) => ActivityResult(
        _l(j['items']).map((e) => ActivityItem.fromJson(_m(e))).toList(),
        _m(j['errors']).map((k, v) => MapEntry(k, _s(v))),
      );
  final List<ActivityItem> items;
  final Map<String, String> errors; // 设备名 → 失败原因
}

/// 实时事件（protocol.Event）。
class CallEvent {
  CallEvent({required this.type, this.tool = '', this.stream = '', this.data = '', this.code = '', this.durationMs = 0, this.droppedBytes = 0});
  factory CallEvent.fromJson(Map<String, dynamic> j) => CallEvent(
        type: _s(j['type']),
        tool: _s(j['tool']),
        stream: _s(j['stream']),
        data: _s(j['data']),
        code: _s(j['code']),
        durationMs: _i(j['duration_ms']),
        droppedBytes: _i(j['dropped_bytes']),
      );
  final String type; // start / output / end
  final String tool;
  final String stream; // stdout / stderr
  final String data;
  final String code;
  final int durationMs;
  final int droppedBytes;
}

/// 工具调用的结果。
class ToolResult {
  ToolResult(this.requestId, this.result, this.durationMs);
  final String requestId;
  final Map<String, dynamic> result;
  final int durationMs;
}

/// 目录条目（fs.list）。
class FileEntry {
  FileEntry({required this.name, required this.type, required this.size, required this.mtime});
  factory FileEntry.fromJson(Map<String, dynamic> j) =>
      FileEntry(name: _s(j['name']), type: _s(j['type']), size: _i(j['size']), mtime: _s(j['mtime']));
  final String name;
  final String type; // file / dir / symlink / other
  final int size;
  final String mtime;
  bool get isDir => type == 'dir';
}
