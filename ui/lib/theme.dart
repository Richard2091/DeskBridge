// 清爽主题（docs/ui-design.md 5.2）的浅色和深色。Material 没有的颜色放在 DeskTokens 里。

import 'package:flutter/material.dart';

@immutable
class DeskTokens extends ThemeExtension<DeskTokens> {
  const DeskTokens({
    required this.page,
    required this.surface,
    required this.surface2,
    required this.surface3,
    required this.border,
    required this.borderStrong,
    required this.text,
    required this.muted,
    required this.faint,
    required this.accent,
    required this.accentSoft,
    required this.ok,
    required this.okSoft,
    required this.warn,
    required this.warnSoft,
    required this.bad,
    required this.badSoft,
    required this.info,
    required this.infoSoft,
    required this.ai,
    required this.aiSoft,
    required this.codeBg,
    required this.codeText,
  });

  final Color page, surface, surface2, surface3, border, borderStrong;
  final Color text, muted, faint;
  final Color accent, accentSoft;
  final Color ok, okSoft, warn, warnSoft, bad, badSoft, info, infoSoft, ai, aiSoft;
  final Color codeBg, codeText;

  static const light = DeskTokens(
    page: Color(0xFFF4F4F6),
    surface: Color(0xFFFFFFFF),
    surface2: Color(0xFFF8F8FA),
    surface3: Color(0xFFF1F1F4),
    border: Color(0xFFE6E6EA),
    borderStrong: Color(0xFFD4D4DA),
    text: Color(0xFF18181B),
    muted: Color(0xFF71717A),
    faint: Color(0xFFA1A1AA),
    accent: Color(0xFF5B5BD6),
    accentSoft: Color(0xFFEDEDFC),
    ok: Color(0xFF16A34A),
    okSoft: Color(0xFFE7F6EC),
    warn: Color(0xFFD97706),
    warnSoft: Color(0xFFFDF1E1),
    bad: Color(0xFFDC2626),
    badSoft: Color(0xFFFCE9E9),
    info: Color(0xFF2563EB),
    infoSoft: Color(0xFFE8EFFD),
    ai: Color(0xFF7C3AED),
    aiSoft: Color(0xFFF1EAFD),
    codeBg: Color(0xFF18181B),
    codeText: Color(0xFFE4E4E7),
  );

  static const dark = DeskTokens(
    page: Color(0xFF0F0F12),
    surface: Color(0xFF17171B),
    surface2: Color(0xFF131316),
    surface3: Color(0xFF1D1D22),
    border: Color(0xFF26262D),
    borderStrong: Color(0xFF34343C),
    text: Color(0xFFEDEDF0),
    muted: Color(0xFF9A9AA5),
    faint: Color(0xFF63636E),
    accent: Color(0xFF7C7CF0),
    accentSoft: Color(0x267C7CF0),
    ok: Color(0xFF34D399),
    okSoft: Color(0x2634D399),
    warn: Color(0xFFFBBF24),
    warnSoft: Color(0x26FBBF24),
    bad: Color(0xFFF87171),
    badSoft: Color(0x26F87171),
    info: Color(0xFF60A5FA),
    infoSoft: Color(0x2660A5FA),
    ai: Color(0xFFA78BFA),
    aiSoft: Color(0x26A78BFA),
    codeBg: Color(0xFF0B0B0D),
    codeText: Color(0xFFE4E4E7),
  );

  @override
  DeskTokens copyWith() => this;

  @override
  DeskTokens lerp(ThemeExtension<DeskTokens>? other, double t) => t < 0.5 ? this : (other as DeskTokens? ?? this);
}

extension DeskContext on BuildContext {
  DeskTokens get tk => Theme.of(this).extension<DeskTokens>()!;
}

const monoFamily = 'Cascadia Mono';
const monoFallback = ['Consolas', 'Noto Sans Mono CJK SC', 'Microsoft YaHei UI', 'monospace'];
const uiFallback = ['Segoe UI Variable', 'Segoe UI', 'Microsoft YaHei UI', 'Noto Sans CJK SC'];

ThemeData buildTheme(Brightness brightness) {
  final tk = brightness == Brightness.light ? DeskTokens.light : DeskTokens.dark;
  final scheme = ColorScheme.fromSeed(seedColor: tk.accent, brightness: brightness).copyWith(
    primary: tk.accent,
    surface: tk.surface,
    onSurface: tk.text,
    error: tk.bad,
    outline: tk.borderStrong,
    outlineVariant: tk.border,
  );
  final base = ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: scheme,
    scaffoldBackgroundColor: tk.page,
    fontFamilyFallback: uiFallback,
    dividerColor: tk.border,
    extensions: [tk],
  );
  return base.copyWith(
    textTheme: base.textTheme.apply(bodyColor: tk.text, displayColor: tk.text),
    dividerTheme: DividerThemeData(color: tk.border, space: 1, thickness: 1),
    inputDecorationTheme: InputDecorationTheme(
      isDense: true,
      filled: true,
      fillColor: tk.surface2,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(8), borderSide: BorderSide(color: tk.borderStrong)),
      enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(8), borderSide: BorderSide(color: tk.borderStrong)),
      focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(8), borderSide: BorderSide(color: tk.accent, width: 1.5)),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 34),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(0, 34),
        side: BorderSide(color: tk.borderStrong),
        foregroundColor: tk.text,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8))),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: tk.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(color: tk.text, borderRadius: BorderRadius.circular(6)),
      textStyle: TextStyle(color: tk.surface, fontSize: 12),
    ),
  );
}
