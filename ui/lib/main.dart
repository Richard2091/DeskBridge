import 'dart:async';

import 'package:flutter/material.dart';

import 'app.dart';
import 'core/app_model.dart';
import 'core/core_process.dart';
import 'core/models.dart';
import 'theme.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const Bootstrap());
}

/// 启动 Go 后台，成功后进入主界面；失败时显示原因和重试。
class Bootstrap extends StatefulWidget {
  const Bootstrap({super.key});
  @override
  State<Bootstrap> createState() => _BootstrapState();
}

class _BootstrapState extends State<Bootstrap> {
  CoreProcess? _core;
  AppModel? _model;
  String? _error;

  @override
  void initState() {
    super.initState();
    _start();
  }

  Future<void> _start() async {
    setState(() => _error = null);
    try {
      final core = await CoreProcess.start();
      if (!mounted) {
        await core.stop();
        return;
      }
      setState(() {
        _core = core;
        _model = AppModel(core.api, configFile: defaultConfigFile(), nodeMode: core.nodeMode);
      });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    }
  }

  @override
  void dispose() {
    _model?.dispose();
    unawaited(_core?.stop());
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final m = _model;
    if (m != null) return DeskBridgeApp(model: m);
    return MaterialApp(
      title: 'DeskBridge',
      debugShowCheckedModeBanner: false,
      theme: buildTheme(Brightness.light),
      darkTheme: buildTheme(Brightness.dark),
      home: Scaffold(
        body: Center(
          child: _error == null
              ? const CircularProgressIndicator()
              : Padding(
                  padding: const EdgeInsets.all(32),
                  child: Column(mainAxisSize: MainAxisSize.min, children: [
                    const Icon(Icons.error_outline, size: 40, color: Color(0xFFDC2626)),
                    const SizedBox(height: 12),
                    const Text('无法启动 DeskBridge 后台', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
                    const SizedBox(height: 8),
                    SelectableText(_error!, textAlign: TextAlign.center),
                    const SizedBox(height: 16),
                    FilledButton(onPressed: _start, child: const Text('重试')),
                  ]),
                ),
        ),
      ),
    );
  }
}
