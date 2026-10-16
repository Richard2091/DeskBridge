// 应用外壳：侧栏加内容区；主题随设置变化。

import 'package:flutter/material.dart';

import 'core/app_model.dart';
import 'pages/activity_page.dart';
import 'pages/device_page.dart';
import 'pages/devices_page.dart';
import 'pages/settings_page.dart';
import 'theme.dart';

class DeskBridgeApp extends StatelessWidget {
  const DeskBridgeApp({super.key, required this.model});
  final AppModel model;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: model,
      builder: (context, _) => MaterialApp(
        title: 'DeskBridge',
        debugShowCheckedModeBanner: false,
        theme: buildTheme(Brightness.light),
        darkTheme: buildTheme(Brightness.dark),
        themeMode: model.themeMode,
        home: Shell(model: model),
      ),
    );
  }
}

class Shell extends StatelessWidget {
  const Shell({super.key, required this.model});
  final AppModel model;

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: model,
      builder: (context, _) {
        final wide = MediaQuery.sizeOf(context).width >= 1000;
        return Scaffold(
          body: Row(children: [
            _Sidebar(model: model, wide: wide),
            VerticalDivider(width: 1, color: context.tk.border),
            Expanded(child: _content()),
          ]),
        );
      },
    );
  }

  Widget _content() {
    switch (model.page) {
      case AppPage.devices:
        final d = model.openDevice;
        if (model.openDeviceTarget != null && d != null) {
          return DevicePage(key: ValueKey(d.target), model: model, device: d);
        }
        return DevicesPage(model: model);
      case AppPage.activity:
        return ActivityPage(model: model);
      case AppPage.settings:
        return SettingsPage(model: model);
    }
  }
}

class _Sidebar extends StatelessWidget {
  const _Sidebar({required this.model, required this.wide});
  final AppModel model;
  final bool wide;

  @override
  Widget build(BuildContext context) {
    final tk = context.tk;
    return Container(
      width: wide ? 196 : 64,
      color: tk.surface,
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 14),
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(6, 2, 6, 18),
          child: Row(mainAxisAlignment: wide ? MainAxisAlignment.start : MainAxisAlignment.center, children: [
            Container(
              width: 28,
              height: 28,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(8),
                gradient: LinearGradient(colors: [tk.accent, tk.ai], begin: Alignment.topLeft, end: Alignment.bottomRight),
              ),
              child: const Icon(Icons.swap_horiz, color: Colors.white, size: 18),
            ),
            if (wide) ...[
              const SizedBox(width: 10),
              const Expanded(child: Text('DeskBridge', overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700))),
            ],
          ]),
        ),
        _item(context, AppPage.devices, Icons.devices_outlined, '设备'),
        _item(context, AppPage.activity, Icons.history, '操作记录', badge: model.runningCount),
        const Spacer(),
        _item(context, AppPage.settings, Icons.settings_outlined, '设置'),
      ]),
    );
  }

  Widget _item(BuildContext context, AppPage page, IconData icon, String label, {int badge = 0}) {
    final tk = context.tk;
    final sel = model.page == page;
    final fg = sel ? tk.accent : tk.muted;
    return Padding(
      padding: const EdgeInsets.only(bottom: 3),
      child: Semantics(
        button: true,
        selected: sel,
        label: badge > 0 ? '$label，$badge 个进行中' : label,
        child: Material(
          color: sel ? tk.accentSoft : Colors.transparent,
          borderRadius: BorderRadius.circular(8),
          child: InkWell(
            borderRadius: BorderRadius.circular(8),
            onTap: () {
              if (page == AppPage.devices && model.page == AppPage.devices) model.closeDevice();
              model.goTo(page);
            },
            child: Padding(
              padding: EdgeInsets.symmetric(horizontal: wide ? 12 : 0, vertical: 10),
              child: Row(mainAxisAlignment: wide ? MainAxisAlignment.start : MainAxisAlignment.center, children: [
                Badge(
                  isLabelVisible: badge > 0 && !wide,
                  label: Text('$badge'),
                  child: Icon(icon, size: 20, color: fg),
                ),
                if (wide) ...[
                  const SizedBox(width: 12),
                  Expanded(child: Text(label, style: TextStyle(color: sel ? tk.accent : tk.text, fontWeight: sel ? FontWeight.w600 : FontWeight.w400))),
                  if (badge > 0)
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 1),
                      decoration: BoxDecoration(color: tk.info, borderRadius: BorderRadius.circular(999)),
                      child: Text('$badge', style: const TextStyle(color: Colors.white, fontSize: 11.5, fontWeight: FontWeight.w600)),
                    ),
                ],
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
