// DeskBridge 客户端界面实测（Flutter 版）。一次性验证代码，不用于生产。
import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import 'core_process.dart';
import 'win_title.dart';

const _addr = '127.0.0.1:47802';
const _base = 'http://$_addr';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const App());
}

class App extends StatelessWidget {
  const App({super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'DeskBridge 实测',
        theme: ThemeData(colorSchemeSeed: Colors.indigo, useMaterial3: true),
        home: const Shell(),
      );
}

/// 根组件：启动 Go 核心，管理登录状态与退出清理。
class Shell extends StatefulWidget {
  const Shell({super.key});
  @override
  State<Shell> createState() => _ShellState();
}

class _ShellState extends State<Shell> {
  final core = CoreProcess(_addr);
  late final AppLifecycleListener _lifecycle;
  bool loggedIn = false;
  String? coreError;
  bool coreReady = false;
  final autoStream = Platform.environment['DESKBRIDGE_AUTOSTREAM'] == '1';

  @override
  void initState() {
    super.initState();
    setWindowTitle('DeskBridge 实测');
    _lifecycle = AppLifecycleListener(
      onExitRequested: () async {
        await core.stop();
        return ui.AppExitResponse.exit;
      },
    );
    _startCore();
  }

  Future<void> _startCore() async {
    try {
      await core.start();
      if (!mounted) return;
      setState(() {
        coreReady = true;
        if (autoStream) loggedIn = true;
      });
    } catch (e) {
      if (mounted) setState(() => coreError = '$e');
    }
  }

  @override
  void dispose() {
    _lifecycle.dispose();
    core.stop();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (coreError != null) {
      return Scaffold(body: Center(child: Text('Go 核心启动失败：$coreError')));
    }
    if (!coreReady) {
      return const Scaffold(body: Center(child: Text('正在启动 Go 核心…')));
    }
    if (!loggedIn) {
      return LoginPage(onLogin: () => setState(() => loggedIn = true));
    }
    return HomePage(autoStream: autoStream);
  }
}

class LoginPage extends StatelessWidget {
  const LoginPage({super.key, required this.onLogin});
  final VoidCallback onLogin;
  @override
  Widget build(BuildContext context) => Scaffold(
        body: Center(
          child: SizedBox(
            width: 320,
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              const Text('DeskBridge', style: TextStyle(fontSize: 28)),
              const SizedBox(height: 24),
              const TextField(decoration: InputDecoration(labelText: '账号')),
              const TextField(
                  obscureText: true,
                  decoration: InputDecoration(labelText: '密码')),
              const SizedBox(height: 24),
              FilledButton(onPressed: onLogin, child: const Text('登录')),
            ]),
          ),
        ),
      );
}

class Device {
  Device(this.name, this.os, this.online, this.conn, this.relay, this.latency);
  factory Device.fromJson(Map<String, dynamic> j) => Device(
      j['name'] as String,
      j['os'] as String,
      j['online'] as bool,
      j['conn'] as String,
      (j['relay'] as String?) ?? '',
      (j['latency_ms'] as num).toInt());
  final String name, os, conn, relay;
  final bool online;
  final int latency;
  String get connText => switch (conn) {
        'lan' => '局域网直连',
        'direct' => '直连',
        'relay' => '中继（$relay）',
        _ => '离线',
      };
}

class HomePage extends StatefulWidget {
  const HomePage({super.key, required this.autoStream});
  final bool autoStream;
  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  final client = HttpClient();
  List<Device> devices = [];
  String? devError;
  final cmdCtl = TextEditingController(text: 'Write-Output 中文测试');
  String out = '', err = '', exitText = '';
  bool running = false;

  @override
  void initState() {
    super.initState();
    _loadDevices();
  }

  @override
  void dispose() {
    client.close(force: true);
    cmdCtl.dispose();
    super.dispose();
  }

  Future<void> _loadDevices() async {
    try {
      final req = await client.getUrl(Uri.parse('$_base/api/devices'));
      final resp = await req.close();
      final body = await utf8.decodeStream(resp);
      final list = (jsonDecode(body) as List).cast<Map<String, dynamic>>();
      if (mounted) setState(() => devices = list.map(Device.fromJson).toList());
    } catch (e) {
      if (mounted) setState(() => devError = '$e');
    }
  }

  Future<void> _exec() async {
    setState(() => running = true);
    try {
      final req = await client.postUrl(Uri.parse('$_base/api/exec'));
      req.headers.contentType = ContentType.json;
      req.add(utf8.encode(jsonEncode({'command': cmdCtl.text})));
      final resp = await req.close();
      final j = jsonDecode(await utf8.decodeStream(resp)) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        out = '${j['stdout'] ?? ''}';
        err = '${j['stderr'] ?? ''}';
        exitText = '${j['exit_code']}（${j['ms']} ms）';
      });
    } catch (e) {
      if (mounted) setState(() => err = '$e');
    } finally {
      if (mounted) setState(() => running = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('DeskBridge 主页')),
      body: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        SizedBox(
          width: 460,
          child: ListView(padding: const EdgeInsets.all(12), children: [
            Row(children: [
              const Text('设备列表', style: TextStyle(fontSize: 18)),
              const Spacer(),
              TextButton(onPressed: _loadDevices, child: const Text('刷新')),
            ]),
            if (devError != null) Text('加载失败：$devError'),
            DataTable(
              columnSpacing: 16,
              columns: const [
                DataColumn(label: Text('名称')),
                DataColumn(label: Text('系统')),
                DataColumn(label: Text('在线')),
                DataColumn(label: Text('连接方式')),
                DataColumn(label: Text('延迟')),
              ],
              rows: [
                for (final d in devices)
                  DataRow(cells: [
                    DataCell(Text(d.name)),
                    DataCell(Text(d.os)),
                    DataCell(Text(d.online ? '是' : '否')),
                    DataCell(Text(d.connText)),
                    DataCell(Text(d.online ? '${d.latency} ms' : '-')),
                  ]),
              ],
            ),
            const Divider(height: 32),
            const Text('执行命令', style: TextStyle(fontSize: 18)),
            const SizedBox(height: 8),
            TextField(
              controller: cmdCtl,
              onSubmitted: (_) => _exec(),
              decoration: const InputDecoration(
                  border: OutlineInputBorder(), labelText: 'PowerShell 命令'),
            ),
            const SizedBox(height: 8),
            FilledButton(
                onPressed: running ? null : _exec, child: const Text('执行')),
            const SizedBox(height: 8),
            Text('退出码：$exitText'),
            const Text('标准输出：'),
            SelectableText(out, style: const TextStyle(fontFamily: 'Consolas')),
            const Text('标准错误：'),
            SelectableText(err, style: const TextStyle(fontFamily: 'Consolas')),
          ]),
        ),
        const VerticalDivider(width: 1),
        Expanded(child: RemoteView(autoStart: widget.autoStream)),
      ]),
    );
  }
}

/// 远程画面：读取「4 字节大端长度 + JPEG」的 HTTP 流，
/// 用 ui.decodeImageFromList 解码成 ui.Image，再用 RawImage 绘制。
/// 解码忙时只保留最新一帧（丢弃旧帧），避免积压。
class RemoteView extends StatefulWidget {
  const RemoteView({super.key, required this.autoStart});
  final bool autoStart;
  @override
  State<RemoteView> createState() => _RemoteViewState();
}

class _RemoteViewState extends State<RemoteView> {
  HttpClient? _client;
  StreamSubscription<List<int>>? _sub;
  bool streaming = false;
  ui.Image? _image;
  double fps = 0;
  int _shown = 0;
  int _received = 0;
  Timer? _timer;
  DateTime _last = DateTime.now();
  String status = '未开始';

  // 帧重组缓冲
  Uint8List _buf = Uint8List(1 << 20);
  int _len = 0;
  bool _decoding = false;
  Uint8List? _pending;

  @override
  void initState() {
    super.initState();
    if (widget.autoStart) {
      WidgetsBinding.instance.addPostFrameCallback((_) => _start());
    }
  }

  @override
  void dispose() {
    _stop();
    _image?.dispose();
    super.dispose();
  }

  Future<void> _start() async {
    if (streaming) return;
    setState(() {
      streaming = true;
      status = '连接中…';
    });
    _shown = 0;
    _received = 0;
    _last = DateTime.now();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) {
      final now = DateTime.now();
      final dt = now.difference(_last).inMilliseconds / 1000.0;
      _last = now;
      final f = dt > 0 ? _shown / dt : 0.0;
      _shown = 0;
      if (!mounted) return;
      setState(() => fps = f);
      setWindowTitle('DeskBridge 实测 fps=${f.toStringAsFixed(1)}');
    });
    try {
      _client = HttpClient();
      final req = await _client!
          .getUrl(Uri.parse('$_base/api/frames?fps=10&width=1280&q=70'));
      final resp = await req.close();
      if (!mounted) return;
      setState(() => status = '推流中');
      _len = 0;
      _sub = resp.listen(_onChunk, onError: (e) => _fail('$e'), onDone: _stop);
    } catch (e) {
      _fail('$e');
    }
  }

  void _fail(String m) {
    _stop();
    if (mounted) setState(() => status = '出错：$m');
  }

  void _stop() {
    _timer?.cancel();
    _timer = null;
    _sub?.cancel();
    _sub = null;
    _client?.close(force: true);
    _client = null;
    if (mounted && streaming) {
      setState(() {
        streaming = false;
        if (!status.startsWith('出错')) status = '已停止';
      });
    }
  }

  void _onChunk(List<int> chunk) {
    final need = _len + chunk.length;
    if (need > _buf.length) {
      final nb = Uint8List(need * 2);
      nb.setRange(0, _len, _buf);
      _buf = nb;
    }
    _buf.setRange(_len, need, chunk);
    _len = need;
    var off = 0;
    while (_len - off >= 4) {
      final n = ByteData.sublistView(_buf, off, off + 4).getUint32(0);
      if (_len - off - 4 < n) break;
      // 拷贝出独立的 JPEG 字节（解码是异步的，不能引用会被复用的缓冲）
      final jpeg = Uint8List.fromList(_buf.sublist(off + 4, off + 4 + n));
      off += 4 + n;
      _received++;
      _submit(jpeg);
    }
    if (off > 0) {
      _buf.setRange(0, _len - off, _buf, off);
      _len -= off;
    }
  }

  void _submit(Uint8List jpeg) {
    if (_decoding) {
      _pending = jpeg; // 只保留最新
      return;
    }
    _decoding = true;
    ui.decodeImageFromList(jpeg, (img) {
      _decoding = false;
      if (!mounted || !streaming) {
        img.dispose();
        return;
      }
      final old = _image;
      setState(() {
        _image = img;
        _shown++;
      });
      if (old != null) {
        WidgetsBinding.instance.addPostFrameCallback((_) => old.dispose());
      }
      final p = _pending;
      if (p != null) {
        _pending = null;
        _submit(p);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Text('远程画面', style: TextStyle(fontSize: 18)),
          const SizedBox(width: 16),
          FilledButton(
              onPressed: streaming ? null : _start, child: const Text('开始')),
          const SizedBox(width: 8),
          OutlinedButton(
              onPressed: streaming ? _stop : null, child: const Text('停止')),
          const SizedBox(width: 16),
          Text('$status　显示帧率：${fps.toStringAsFixed(1)} fps（已收 $_received 帧）'),
        ]),
        const SizedBox(height: 8),
        Expanded(
          child: Container(
            color: Colors.black,
            width: double.infinity,
            child: _image == null
                ? const SizedBox.shrink()
                : RawImage(image: _image, fit: BoxFit.contain),
          ),
        ),
      ]),
    );
  }
}