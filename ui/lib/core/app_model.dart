// 界面的全局状态：后台连接、设备列表、操作记录、导航和主题。
// 设备和记录用定时轮询刷新（节点没有全局推送，实时输出才用 WebSocket）。

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';

import 'api.dart';
import 'models.dart';

enum AppPage { devices, activity, settings }

class AppModel extends ChangeNotifier {
  AppModel(this.api, {this.configFile, bool poll = true}) {
    _loadConfig();
    if (poll) startPolling();
  }

  final Api api;
  final File? configFile;

  // ---- 数据 ----
  LocalStatus? status;
  DeviceList? devices;
  ActivityResult? activity;
  String? devicesError; // 读取设备列表失败的原因（含本机节点未运行）
  String? activityError;
  bool devicesLoading = false;

  int get runningCount => activity?.items.where((e) => e.running).length ?? 0;

  // ---- 导航 ----
  AppPage page = AppPage.devices;
  String? openDeviceTarget; // 设备页：正在查看的设备（Device.target）
  String? selectedRequestId; // 操作记录页：选中的记录

  Device? get openDevice {
    final t = openDeviceTarget;
    if (t == null) return null;
    for (final g in devices?.groups ?? const <DeviceGroup>[]) {
      for (final d in g.devices) {
        if (d.target == t) return d;
      }
    }
    return null;
  }

  void goTo(AppPage p) {
    page = p;
    notifyListeners();
  }

  void openDeviceNamed(Device d) {
    page = AppPage.devices;
    openDeviceTarget = d.target;
    notifyListeners();
  }

  void closeDevice() {
    openDeviceTarget = null;
    notifyListeners();
  }

  void openActivityItem(String requestId) {
    page = AppPage.activity;
    selectedRequestId = requestId;
    notifyListeners();
    refreshActivity();
  }

  void selectActivity(String? requestId) {
    selectedRequestId = requestId;
    notifyListeners();
  }

  // ---- 主题 ----
  ThemeMode themeMode = ThemeMode.system;

  void setThemeMode(ThemeMode m) {
    themeMode = m;
    notifyListeners();
    _saveConfig();
  }

  void _loadConfig() {
    try {
      final f = configFile;
      if (f == null || !f.existsSync()) return;
      final j = jsonDecode(f.readAsStringSync()) as Map<String, dynamic>;
      themeMode = ThemeMode.values.firstWhere((m) => m.name == j['theme'], orElse: () => ThemeMode.system);
    } catch (_) {}
  }

  void _saveConfig() {
    try {
      final f = configFile;
      if (f == null) return;
      f.parent.createSync(recursive: true);
      f.writeAsStringSync(jsonEncode({'theme': themeMode.name}));
    } catch (_) {}
  }

  // ---- 轮询 ----
  Timer? _devTimer, _actTimer, _stTimer;
  bool _devBusy = false, _actBusy = false, _stBusy = false, _disposed = false;

  void startPolling() {
    refreshDevices();
    refreshActivity();
    refreshStatus();
    _devTimer = Timer.periodic(const Duration(seconds: 8), (_) => refreshDevices());
    _actTimer = Timer.periodic(const Duration(seconds: 3), (_) => refreshActivity());
    _stTimer = Timer.periodic(const Duration(seconds: 10), (_) => refreshStatus());
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  Future<void> refreshStatus() async {
    if (_stBusy) return;
    _stBusy = true;
    try {
      status = await api.status();
    } on ApiException {
      // 状态取不到时设备列表那边会给出原因。
    } finally {
      _stBusy = false;
      _notify();
    }
  }

  Future<void> refreshDevices({bool force = false}) async {
    if (_devBusy) return;
    _devBusy = true;
    devicesLoading = devices == null || force;
    _notify();
    try {
      devices = await api.devices(refresh: force);
      devicesError = null;
    } on ApiException catch (e) {
      devicesError = e.code == 'node_not_running' ? '本机节点未运行。请先启动 DeskBridge 节点（deskbridge node），或运行 deskbridge install 设为开机自启。' : e.message;
    } finally {
      _devBusy = false;
      devicesLoading = false;
      _notify();
    }
  }

  Future<void> refreshActivity() async {
    if (_actBusy) return;
    _actBusy = true;
    try {
      activity = await api.activity();
      activityError = null;
    } on ApiException catch (e) {
      activityError = e.message;
    } finally {
      _actBusy = false;
      _notify();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _devTimer?.cancel();
    _actTimer?.cancel();
    _stTimer?.cancel();
    super.dispose();
  }
}

/// 配置文件位置：%APPDATA%\DeskBridge\ui.json（Windows）或 ~/.config/deskbridge/ui.json。
File? defaultConfigFile() {
  final env = Platform.environment;
  final String? dir;
  if (Platform.isWindows) {
    final a = env['APPDATA'];
    dir = a == null ? null : '$a\\DeskBridge';
  } else {
    final h = env['HOME'];
    dir = h == null ? null : '$h/.config/deskbridge';
  }
  if (dir == null) return null;
  return File('$dir${Platform.pathSeparator}ui.json');
}
