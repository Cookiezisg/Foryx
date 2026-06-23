/// Theme-INVARIANT design tokens — geometry + time values that don't change between light and
/// dark (colors live in [AnColors], a ThemeExtension, so they can lerp). Three self-consistent
/// maths keep every surface dimensionally coherent: density = 4-grid, layout = 2:3:6 harmonic
/// columns, motion = a small fixed duration/easing set. NEVER inline a raw px/ms — read a token.
///
/// 主题无关 token:明暗不变的几何/时间值(会变的色在 [AnColors])。三套自洽数学(密度=4 网格 ·
/// 布局=2:3:6 谐波列 · 动效=固定时长/缓动)保证全局尺寸一致——绝不内联裸 px/ms。
library;

import 'package:flutter/widgets.dart';

/// Spacing scale (4-grid). Value-named to stay unambiguous at call sites.
/// 间距阶梯(4 网格)。值命名,调用处零歧义。
abstract final class AnSpace {
  static const double s2 = 2;
  static const double s4 = 4;
  static const double s6 = 6; // gap-tight: low-weight inline gap (icon↔label, dot↔label) 紧凑行内间距
  static const double s8 = 8; // inline gap + the shell's island padding/gap 行内间距 + 岛内距/间距
  static const double s12 = 12;
  static const double s16 = 16;
  static const double s24 = 24;
  static const double s32 = 32;
  static const double s48 = 48;
  static const double s64 = 64;
}

/// Corner radii (4-grid). Each tier maps to a surface class: tag→button→chip→card→island.
/// 圆角(4 网格)。每级对应一类表面。
abstract final class AnRadius {
  static const double tag = 4;
  static const double button = 8;
  static const double chip = 12;
  static const double card = 16;
  static const double island = 20;
  static const double pill = 999;
}

/// Sizes — control heights, icon slots, the 2:3:6 layout columns, the window envelope, and the
/// window-controls reserve. 尺寸——控件高、图标槽、2:3:6 布局列、窗体外廓、窗控留位。
abstract final class AnSize {
  // Density anchors. 密度锚。
  static const double row = 32; // standard row height (the one) 标准行高(唯一)
  static const double control = 28;
  static const double controlSm = 24;
  static const double icon = 16;
  static const double iconSm = 12;
  static const double iconLg = 20;
  static const double dot = 7;
  static const double dotPulse = 5; // run-status breath expansion radius 呼吸外扩半径
  static const double hairline = 1;
  static const double gripLine = 2; // drag-handle hover divider (2× hairline) 拖柄悬停分隔线
  static const double caret = 1.5; // text caret width 文本光标宽
  static const double caretHeight = 16; // text caret height — under the 18.2 body line-height so the cursor hugs the text 文本光标高(小于正文行高 18.2、贴合文字)
  static const double caretEndPad = 3; // end-of-line caret room (caret width + a hair) so the last glyph isn't clipped under the cursor (flutter#24612) 行尾光标留位(光标宽+一丝)

  // Primitive control metrics (the demo's PRIMITIVE METRICS group). 原语控件度量。
  static const double btnPadX = 14; // text-button horizontal optical pad 文本钮水平光学内距
  static const double btnPadXSm = 10; // small-button horizontal pad 小钮水平内距
  static const double badge = 22; // status/tag badge visual height 徽章视觉高度
  static const double badgePadX = 9; // badge horizontal pad 徽章水平内距
  static const double inputMin = 180; // single-line input min width 单行输入最小宽
  static const double inlineEditMin = 32; // in-place edit field min width — an empty seamless field has ~0 intrinsic width and would be un-clickable 就地编辑框最小宽(空 seamless 框固有宽≈0、否则不可点)
  static const double stateIcon = 40; // AnState placeholder glyph — larger than iconLg(20), distinct from control icons 状态占位大字形
  static const double stateMaxWidth = 360; // AnState centered content column max width (short lines stay readable) 状态内容列最大宽
  static const double stepCurrent = 18; // AnStepper elongated current-dot width (done/upcoming use dot=7) 步进器当前点拉长宽
  static const double block = 280; // inspector 2-col min track + badge max-width 检查器列 + 徽章最大宽
  static const double menuMinWidth = 200; // dropdown/menu min width (rich rows fit even off a compact trigger) 菜单最小宽(紧凑触发器也容得下富行)
  static const double menuMaxWidth = 360; // dropdown/menu popover max width 菜单浮层最大宽
  static const double menuMaxHeight = 320; // dropdown/menu popover max height (then scrolls) 菜单浮层最大高(超则滚)

  // Three-island layout columns. The LEFT island is elastic (draggable, 240–400, default 320);
  // the RIGHT island is fixed; the ocean is the flex remainder whose content column is elastic
  // 480–720 (`oceanMin`..`content`). 三岛列:左岛弹性(可拖 240–400,默认 320);右岛固定;
  // 海洋取余量、内容列弹性 480–720。
  static const double sidebar = 320; // left island default 左岛默认
  static const double sidebarMin = 240; // left island min (drag) 左岛最小
  static const double sidebarMax = 400; // left island max (drag) 左岛最大
  static const double rightIsland = 320; // right island — FIXED 右岛固定宽
  static const double content = 720; // 6u · ocean content column MAX (centers when wider) 内容列最大(更宽则居中)
  static const double oceanMin = 480; // ocean content column MIN (elastic 480–720) 内容列最小(弹性 480–720)
  static const double islandHead = 44; // floating header height 浮动头高

  // Shell envelope: 8px padding around the islands + 8px gaps between them.
  // 壳外廓:岛四周 8px 内距 + 岛间 8px 间距。
  static const double shellPad = AnSpace.s8;
  static const double shellGap = AnSpace.s8;

  // Window minimum — in LOGICAL POINTS. Sized to GUARANTEE the ocean keeps its minimum content
  // column (`oceanMin` = 480) even with the left island dragged to its MAX (worst case):
  // pad + sidebarMax(400) + gap + oceanMin(480) + gap + rightIsland(320) + pad = 1232. Min HEIGHT
  // = golden-ratio complement. Comfortably fits a 1512pt laptop with margin.
  // 窗口最小(逻辑点):保证即便左岛拖到最大(worst case)、海洋仍有最小内容列 480 =
  // 8+400+8+480+8+320+8 = 1232。高=黄金比例补。1512 屏上留有余量。
  static const double goldenRatio = 1.618;
  static const double windowMinWidth =
      shellPad + sidebarMax + shellGap + oceanMin + shellGap + rightIsland + shellPad; // 1232
  static const double windowMinHeight = windowMinWidth / goldenRatio; // ≈ 761
  static const double windowInitialWidth = 1280; // comfortable default, margin on a 1512pt screen 舒适默认、留余量
  static const double windowInitialHeight = windowInitialWidth / goldenRatio; // ≈ 791

  // The left-island chrome bar reserves this horizontal room for the macOS traffic lights, which
  // the OS draws/centers in the (taller) title bar — see window_setup (addToolbar). The lights'
  // VERTICAL position is OS-managed (click-safe); we never reposition the native buttons.
  // 左岛 chrome 条给红绿灯留此横向位;灯由 OS 在(加高的)标题栏绘制居中(见 window_setup 的 addToolbar),
  // 纵向位置 OS 托管、点击安全;绝不手动挪原生按钮。
  static const double windowControlsInset = 72;
}

/// Opacity tokens — the few semantic alpha values used as whole-widget dimmers. 整件透明度语义值。
abstract final class AnOpacity {
  static const double disabled = 0.4; // dimmed disabled controls 禁用控件变暗
}

/// Motion — durations + easing. fast = hover, mid = reveals, slow = island slides; breath is
/// the run-status pulse. 动效:fast 悬停 / mid 揭示 / slow 岛屿滑动;breath 运行呼吸。
abstract final class AnMotion {
  static const Duration fast = Duration(milliseconds: 120);
  static const Duration mid = Duration(milliseconds: 240);
  static const Duration slow = Duration(milliseconds: 340);
  static const Duration breath = Duration(milliseconds: 1800);

  static const Cubic easeOut = Cubic(0.16, 1, 0.3, 1);
  static const Cubic spring = Cubic(0.2, 0.9, 0.25, 1);
}

/// Accessibility-driven motion gate — the single source every animated An* widget reads in build()
/// to decide whether to run. Uses the ASPECT accessors so a widget rebuilds only when the flag
/// flips, never on unrelated MediaQuery changes (NEVER read raw `MediaQuery.of(c).disableAnimations`
/// — over-rebuilds — and never per-platform detection). [reduced] gates FUNCTIONAL one-shot reveals;
/// [reducedOrAssistive] gates DECORATIVE loops (shimmer / caret blink / typewriter / breath pulse) —
/// continuous motion under an active screen reader is noise that competes with announcements.
///
/// 无障碍动效门控——每个动画 An* 件 build() 里读它决定要不要动。用 aspect 访问器(只在标志翻转时 rebuild)。
/// reduced 门控功能性一次性揭示;reducedOrAssistive 门控装饰循环(屏幕阅读器活跃时持续动效是噪声)。
abstract final class AnMotionPref {
  static bool reduced(BuildContext context) => MediaQuery.disableAnimationsOf(context);
  static bool reducedOrAssistive(BuildContext context) =>
      MediaQuery.disableAnimationsOf(context) || MediaQuery.accessibleNavigationOf(context);
}
