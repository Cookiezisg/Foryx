import 'package:flutter/widgets.dart';

import '../../i18n/strings.g.dart';
import '../design/colors.dart';
import '../design/tokens.dart';

/// C4 — a loading skeleton: muted bones shaped like the content to come (row / card / text / lines)
/// with a single restrained shimmer sweep. HAND-ROLL (the `shimmer` package is stale and still makes
/// you build every shape; `skeletonizer` is a whole layout-introspection engine we'd bypass to use
/// one effect — principle #8 "don't add a package when a primitive composes"). ONE
/// SingleTicker controller drives ONE [ShaderMask] (BlendMode.srcATop, so the sweep paints only over
/// the opaque bones) whose gradient is slid by a [GradientTransform]; the whole thing sits in a
/// [RepaintBoundary] so the per-frame sweep never dirties siblings.
///
/// Reduced-motion (gated on [AnMotionPref.reducedOrAssistive] — a shimmer is a decorative loop that
/// competes with a screen reader): render the SAME bones at the flat [AnColors.skeletonBase] with NO
/// controller running — a calm finished block, not a degraded one (动效克制). One [Semantics]
/// announces "loading" politely; the bones are decorative ([ExcludeSemantics]).
///
/// C4——加载骨架:哑底骨头按将来内容塑形(row/card/text/lines)+ 克制扫光。HAND-ROLL。一个 SingleTicker 控制器
/// 驱动一个 ShaderMask(srcATop:扫光只覆盖不透明骨头),整体裹 RepaintBoundary。降级(reducedOrAssistive)下渲染
/// 同骨头、纯 skeletonBase、不跑控制器。一个 Semantics 播报 loading,骨头装饰(ExcludeSemantics)。
enum _Shape { row, card, text, lines }

class AnSkeleton extends StatefulWidget {
  const AnSkeleton.row({super.key})
      : _shape = _Shape.row,
        _lines = 0;
  const AnSkeleton.card({super.key})
      : _shape = _Shape.card,
        _lines = 0;
  const AnSkeleton.text({super.key})
      : _shape = _Shape.text,
        _lines = 0;
  const AnSkeleton.lines(int count, {super.key})
      : _shape = _Shape.lines,
        _lines = count;

  final _Shape _shape;
  final int _lines;

  @override
  State<AnSkeleton> createState() => _AnSkeletonState();
}

class _AnSkeletonState extends State<AnSkeleton> with SingleTickerProviderStateMixin {
  // EAGER-INIT (assign in initState, never a lazy `late final =` field). 急切初始化。
  late final AnimationController _c;

  @override
  void initState() {
    super.initState();
    _c = AnimationController(vsync: this, duration: AnMotion.breath);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _sync(); // reduced-motion lives in MediaQuery → start/stop here so a runtime toggle is honoured 降级标志在 MediaQuery
  }

  void _sync() {
    if (AnMotionPref.reducedOrAssistive(context)) {
      _c.stop();
      _c.value = 0;
    } else if (!_c.isAnimating) {
      _c.repeat();
    }
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final bones = _bones(c);
    return Semantics(
      container: true,
      liveRegion: true,
      label: context.t.feedback.loading,
      child: ExcludeSemantics(
        child: RepaintBoundary(
          child: AnMotionPref.reducedOrAssistive(context)
              ? bones // static fallback: flat skeletonBase, no sweep 静态兜底
              : AnimatedBuilder(
                  animation: _c,
                  // Linear sweep (NOT eased): easeOut on a .repeat() controller stutters at the loop
                  // boundary; a constant glide is the smooth shimmer norm. 线性扫光(repeat 上 ease 会在循环点抖)。
                  builder: (ctx, child) => ShaderMask(
                    blendMode: BlendMode.srcATop,
                    shaderCallback: (rect) => _sweep(c, _c.value).createShader(rect),
                    child: child,
                  ),
                  child: bones,
                ),
        ),
      ),
    );
  }

  LinearGradient _sweep(AnColors c, double v) => LinearGradient(
        colors: [c.skeletonBase, c.skeletonHighlight, c.skeletonBase],
        stops: const [0.35, 0.5, 0.65],
        begin: Alignment.centerLeft,
        end: Alignment.centerRight,
        tileMode: TileMode.clamp,
        transform: _SweepTransform(v),
      );

  Widget _bones(AnColors c) {
    switch (widget._shape) {
      case _Shape.text:
        return _bar(c, height: AnSize.skeletonLine);
      case _Shape.lines:
        final n = widget._lines <= 0 ? 1 : widget._lines;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            for (var i = 0; i < n; i++) ...[
              if (i > 0) const SizedBox(height: AnSpace.s8),
              i == n - 1 && n > 1 ? _frac(0.6, _bar(c, height: AnSize.skeletonLine)) : _bar(c, height: AnSize.skeletonLine),
            ],
          ],
        );
      case _Shape.row:
        return Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            _circle(c, AnSize.icon),
            const SizedBox(width: AnSpace.s8),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                children: [
                  _bar(c, height: AnSize.skeletonLine),
                  const SizedBox(height: AnSpace.s6),
                  _frac(0.7, _bar(c, height: AnSize.skeletonLine)),
                ],
              ),
            ),
          ],
        );
      case _Shape.card:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            _bar(c, height: AnSpace.s48, radius: AnRadius.card), // the card block / thumbnail
            const SizedBox(height: AnSpace.s12),
            _frac(0.5, _bar(c, height: AnSize.skeletonLine)), // title
            const SizedBox(height: AnSpace.s8),
            _bar(c, height: AnSize.skeletonLine), // meta 1
            const SizedBox(height: AnSpace.s6),
            _frac(0.8, _bar(c, height: AnSize.skeletonLine)), // meta 2
          ],
        );
    }
  }

  // Bones are OPAQUE (skeletonBase) — srcATop paints the sweep only over opaque pixels, and the base
  // == the gradient's base so a no-highlight bone is unchanged. 骨头不透明,扫光只覆盖之。
  Widget _bar(AnColors c, {required double height, double? radius}) => Container(
        height: height,
        decoration: BoxDecoration(color: c.skeletonBase, borderRadius: BorderRadius.circular(radius ?? AnRadius.tag)),
      );

  Widget _circle(AnColors c, double size) =>
      Container(width: size, height: size, decoration: BoxDecoration(color: c.skeletonBase, shape: BoxShape.circle));

  Widget _frac(double factor, Widget child) =>
      FractionallySizedBox(alignment: Alignment.centerLeft, widthFactor: factor, child: child);
}

// Slides the gradient left→right across the bounds as v goes 0→1 (translate −w → +w); tileMode.clamp
// keeps everything outside the band at the base colour. 随 v 0→1 把渐变从左滑到右,带外 clamp 成 base 色。
class _SweepTransform extends GradientTransform {
  const _SweepTransform(this.v);

  final double v;

  @override
  Matrix4? transform(Rect bounds, {TextDirection? textDirection}) =>
      Matrix4.translationValues(bounds.width * (2 * v - 1), 0, 0);
}
