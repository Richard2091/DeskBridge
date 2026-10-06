import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

// 直接调用 user32 修改本进程 Flutter 顶层窗口的标题，不引入 window_manager。
final _user32 = DynamicLibrary.open('user32.dll');
final _kernel32 = DynamicLibrary.open('kernel32.dll');

final _findWindowEx = _user32.lookupFunction<
    IntPtr Function(IntPtr, IntPtr, Pointer<Utf16>, Pointer<Utf16>),
    int Function(int, int, Pointer<Utf16>, Pointer<Utf16>)>('FindWindowExW');
final _getWindowThreadProcessId = _user32.lookupFunction<
    Uint32 Function(IntPtr, Pointer<Uint32>),
    int Function(int, Pointer<Uint32>)>('GetWindowThreadProcessId');
final _setWindowText = _user32.lookupFunction<
    Int32 Function(IntPtr, Pointer<Utf16>),
    int Function(int, Pointer<Utf16>)>('SetWindowTextW');
final _getCurrentProcessId =
    _kernel32.lookupFunction<Uint32 Function(), int Function()>(
        'GetCurrentProcessId');

int _hwnd = 0;

int _findMyWindow() {
  final cls = 'FLUTTER_RUNNER_WIN32_WINDOW'.toNativeUtf16();
  final pidOut = calloc<Uint32>();
  final me = _getCurrentProcessId();
  var h = 0;
  var found = 0;
  while ((h = _findWindowEx(0, h, cls, nullptr)) != 0) {
    _getWindowThreadProcessId(h, pidOut);
    if (pidOut.value == me) {
      found = h;
      break;
    }
  }
  calloc.free(cls);
  calloc.free(pidOut);
  return found;
}

void setWindowTitle(String title) {
  if (!Platform.isWindows) return;
  if (_hwnd == 0) _hwnd = _findMyWindow();
  if (_hwnd == 0) return; // 窗口还没创建，下次再试
  final s = title.toNativeUtf16();
  _setWindowText(_hwnd, s);
  calloc.free(s);
}