import 'package:flutter/material.dart';

/// Typography — a modular scale anchored on a 13px UI body. [uiFamily] = BUNDLED MiSans, a variable
/// font (wght axis 150–700) covering Latin + Simplified Chinese, so the bilingual UI renders the
/// same on every machine (the demo's intent). We render it LIGHT — body at Light (w300) — to shed
/// the heavy look MiSans has at Regular (ExtraLight 200 was too thin for some glyphs); the ramp
/// climbs from there. Colorless on purpose: the theme applies ink once and it inherits.
///
/// 字体——模数阶梯,锚 13px 正文。[uiFamily]=**随包 MiSans**(变量字体,wght 150–700,Latin+简中),每台机器同字面
/// (demo 本意)。整体**压细**——正文 Light(w300)(ExtraLight 200 部分字偏细);字重阶梯由此上爬。
abstract final class AnText {
  static const String uiFamily = 'MiSans'; // BUNDLED VF (assets/fonts/MiSansVF.ttf), rendered light 随包变量字体,渲染压细
  static const List<String> uiFallback = [
    'PingFang SC', 'Microsoft YaHei', 'Segoe UI', 'Noto Sans', 'sans-serif',
  ];
  static const String monoFamily = 'JetBrains Mono'; // BUNDLED (assets/fonts) — deterministic code face 随包,代码字面确定
  static const List<String> monoFallback = [
    'SF Mono', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace',
  ];

  // Weight ramp anchored on ExtraLight body (w200). MiSans VF maps FontWeight → its wght axis.
  // 字重阶梯锚在 ExtraLight 正文(w200);MiSans 变量字体把 FontWeight 映射到 wght 轴。
  static const TextStyle h1 = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 32, height: 1.25, fontWeight: FontWeight.w500, letterSpacing: -0.5,
  );
  static const TextStyle h2 = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 24, height: 1.25, fontWeight: FontWeight.w500, letterSpacing: -0.3,
  );
  static const TextStyle h3 = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 20, height: 1.3, fontWeight: FontWeight.w500, letterSpacing: -0.2,
  );
  static const TextStyle strong = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 16, height: 1.4, fontWeight: FontWeight.w400, // emphasis = Regular over ExtraLight body 强调=Regular
  );
  static const TextStyle body = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 13, height: 1.4, fontWeight: FontWeight.w300, // the UI anchor — Light 正文锚·Light
  );
  static const TextStyle label = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 13, height: 1.4, fontWeight: FontWeight.w300, // Light 标签·Light
  );
  static const TextStyle meta = TextStyle(
    fontFamily: uiFamily, fontFamilyFallback: uiFallback,
    fontSize: 12, height: 1.4, fontWeight: FontWeight.w300, // muted secondary — Light for small-size legibility 次级·Light(小字可读)
  );
  static const TextStyle mono = TextStyle(
    fontFamily: monoFamily, fontFamilyFallback: monoFallback,
    fontSize: 13, height: 1.5, fontWeight: FontWeight.w400,
  );

  /// Map the scale onto Material's [TextTheme] and bake the ink color in once.
  /// 把字阶映射到 Material [TextTheme] 并一次性注入墨色。
  static TextTheme textTheme(Color ink) => TextTheme(
        displayLarge: h1,
        displayMedium: h1,
        headlineLarge: h2,
        headlineMedium: h2,
        headlineSmall: h3,
        titleLarge: strong,
        titleMedium: strong,
        bodyLarge: body,
        bodyMedium: body,
        bodySmall: meta,
        labelLarge: label,
        labelMedium: label,
        labelSmall: meta,
      ).apply(bodyColor: ink, displayColor: ink);
}
