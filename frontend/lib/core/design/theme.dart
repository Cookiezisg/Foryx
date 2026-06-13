import 'package:flutter/material.dart';

import 'tokens.dart';

/// The app ThemeData built from [Tokens]. One light theme for now (the visual soul is
/// bright/airy); a dark variant is a follow-up. Widgets read colors/metrics from the
/// theme or [Tokens], never hardcode them.
///
/// 由 [Tokens] 构建的 app ThemeData。当前单一明亮主题(视觉灵魂明亮通透);暗色后续。
/// widget 从主题或 [Tokens] 读颜色/度量,绝不硬编码。
abstract final class ForgifyTheme {
  static ThemeData light() {
    final scheme = ColorScheme.fromSeed(
      seedColor: Tokens.accent,
      brightness: Brightness.light,
      surface: Tokens.surface,
    );
    return ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      scaffoldBackgroundColor: Tokens.surface,
      dividerColor: Tokens.border,
      visualDensity: VisualDensity.compact,
      textTheme: const TextTheme().apply(
        bodyColor: Tokens.textPrimary,
        displayColor: Tokens.textPrimary,
      ),
      dividerTheme: const DividerThemeData(
        color: Tokens.border,
        thickness: 1,
        space: 1,
      ),
    );
  }
}
