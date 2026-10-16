// 与桥接进程（deskbridge ui-core）通信：REST 加 WebSocket，全部带令牌。

import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'models.dart';

/// 界面发出的调用都声明为“人、经界面”，操作记录据此区分 AI 和人。
const _initiator = {'type': 'human', 'via': 'ui'};

class Api {
  Api(this.host, this.port, this.token) : _http = HttpClient() {
    _http.connectionTimeout = const Duration(seconds: 5);
    _http.idleTimeout = const Duration(seconds: 30);
  }

  final String host;
  final int port;
  final String token;
  final HttpClient _http;

  void close() => _http.close(force: true);

  Uri _uri(String path, [Map<String, String>? query]) =>
      Uri(scheme: 'http', host: host, port: port, path: path, queryParameters: query);

  /// 发一次请求并解析 JSON。错误响应（{"error":{...}}）转成 [ApiException]。
  Future<Map<String, dynamic>> _send(
    String method,
    Uri uri, {
    Object? body,
    Duration timeout = const Duration(seconds: 30),
  }) async {
    try {
      final req = await _http.openUrl(method, uri).timeout(timeout);
      req.headers.set('Authorization', 'Bearer $token');
      if (body != null) {
        req.headers.contentType = ContentType.json;
        req.add(utf8.encode(jsonEncode(body)));
      }
      final resp = await req.close().timeout(timeout);
      final text = await resp.transform(utf8.decoder).join().timeout(timeout);
      Map<String, dynamic> json = const {};
      if (text.isNotEmpty) {
        try {
          final decoded = jsonDecode(text);
          if (decoded is Map<String, dynamic>) json = decoded;
        } catch (_) {}
      }
      if (resp.statusCode != 200) {
        final err = json['error'];
        if (err is Map<String, dynamic>) {
          throw ApiException('${err['code'] ?? 'internal'}', '${err['message'] ?? '请求失败'}');
        }
        throw ApiException('http_${resp.statusCode}', '请求失败（HTTP ${resp.statusCode}）');
      }
      return json;
    } on ApiException {
      rethrow;
    } on TimeoutException {
      throw ApiException('timeout', '等待本机节点响应超时');
    } on SocketException catch (e) {
      throw ApiException('ui_core_unreachable', '无法连接后台进程：${e.message}');
    }
  }

  Future<LocalStatus> status() async =>
      LocalStatus.fromJson(await _send('GET', _uri('/local/status')));

  Future<DeviceList> devices({bool refresh = false}) async => DeviceList.fromJson(
      await _send('GET', _uri('/local/devices', refresh ? {'refresh': '1'} : null), timeout: const Duration(seconds: 45)));

  Future<ChannelStatus> login(String kind) async =>
      ChannelStatus.fromJson(await _send('POST', _uri('/local/channels/$kind/login')));

  Future<void> logout(String kind) async {
    await _send('POST', _uri('/local/channels/$kind/logout'));
  }

  Future<ActivityResult> activity({String operatorType = '', int limit = 200}) async {
    final q = <String, String>{'limit': '$limit'};
    if (operatorType.isNotEmpty) q['operator_type'] = operatorType;
    return ActivityResult.fromJson(await _send('GET', _uri('/api/activity', q), timeout: const Duration(seconds: 30)));
  }

  /// 生成 request_id（和 Go 端一样：16 字节随机数的十六进制）。
  static String newRequestId() {
    final r = Random.secure();
    return List.generate(16, (_) => r.nextInt(256).toRadixString(16).padLeft(2, '0')).join();
  }

  /// 调用 [target] 设备上的工具。[timeoutMs] 为 0 时用工具默认值。
  Future<ToolResult> callTool(
    String target,
    String tool,
    Map<String, dynamic>? args, {
    String? requestId,
    int timeoutMs = 0,
  }) async {
    final id = requestId ?? newRequestId();
    final wait = Duration(milliseconds: (timeoutMs > 0 ? timeoutMs : 600000) + 15000);
    final json = await _send(
      'POST',
      _uri('/local/devices/${Uri.encodeComponent(target)}/v1/tools/$tool'),
      body: {
        'request_id': id,
        'args': args,
        if (timeoutMs > 0) 'timeout_ms': timeoutMs,
        'initiator': _initiator,
      },
      timeout: wait,
    );
    final result = json['result'];
    return ToolResult(
      id,
      result is Map<String, dynamic> ? result : const {},
      (json['duration_ms'] as num?)?.toInt() ?? 0,
    );
  }

  Future<void> cancel(String target, String requestId) async {
    await _send('DELETE', _uri('/local/devices/${Uri.encodeComponent(target)}/v1/requests/$requestId'));
  }

  /// 订阅一条正在执行的调用的实时事件。调用已结束或不存在时抛出 not_found。
  /// 流在收到 end、连接断开或取消订阅后结束。
  Future<Stream<CallEvent>> events(String target, String requestId) async {
    final uri = Uri(
      scheme: 'ws',
      host: host,
      port: port,
      path: '/local/devices/${Uri.encodeComponent(target)}/v1/requests/$requestId/events',
    );
    try {
      final ws = await WebSocket.connect(uri.toString(), headers: {'Authorization': 'Bearer $token'})
          .timeout(const Duration(seconds: 15));
      return ws
          .map((m) => m is String ? jsonDecode(m) : null)
          .where((m) => m is Map<String, dynamic>)
          .map((m) => CallEvent.fromJson(m as Map<String, dynamic>))
          .asBroadcastStream();
    } on WebSocketException catch (e) {
      // Dart 的握手失败不带状态码。用同一地址发一次普通请求，拿节点的 JSON 错误
      // （调用不存在 → not_found，不是同账号 → permission_denied）。
      try {
        await _send('GET', Uri(scheme: 'http', host: host, port: port, path: uri.path));
      } on ApiException catch (inner) {
        if (inner.code == 'not_found' || inner.code == 'permission_denied' || inner.code == 'unauthorized') rethrow;
      }
      throw ApiException('ws_failed', '订阅实时输出失败：${e.message}');
    } on TimeoutException {
      throw ApiException('timeout', '订阅实时输出超时');
    }
  }
}
