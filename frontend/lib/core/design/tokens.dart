import 'package:flutter/widgets.dart';

/// Design tokens — the single source of the visual soul: bright, airy, light, compact.
/// Densities and the 32px row height (CLAUDE.md 前端守则) live here, never inlined.
///
/// 设计 token——视觉灵魂的单一来源:明亮、通透、轻盈、紧凑。密度与 32px 行高(前端守则)
/// 在此,绝不内联散落。
abstract final class Tokens {
  // Density. 紧凑密度。
  static const double rowHeight = 32;
  static const double gap = 8;
  static const double gapLg = 16;
  static const double radius = 8;
  static const double navRailWidth = 64;
  static const double listPaneWidth = 280;

  // Palette — light, low-chroma surfaces with one calm accent. 明亮低彩度表面 + 一个沉静强调色。
  static const Color accent = Color(0xFF4F46E5); // indigo
  static const Color surface = Color(0xFFFFFFFF);
  static const Color surfaceMuted = Color(0xFFF6F7F9);
  static const Color border = Color(0xFFE6E8EC);
  static const Color textPrimary = Color(0xFF1A1C20);
  static const Color textMuted = Color(0xFF6B7280);
  static const Color danger = Color(0xFFDC2626);
  static const Color ok = Color(0xFF16A34A);
}
