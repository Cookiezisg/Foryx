import 'package:flutter_riverpod/flutter_riverpod.dart';

/// The right island's reveal state — a SEPARATE concern from which entity is bound (that follows the
/// selection). Collapsed is a sticky user preference: selecting a new entity re-binds the terminal but
/// does NOT force the panel back open (better than the demo's re-open-on-every-select; the floating-head
/// panel-right button + the verb CTA re-open it). The shell reveals the right island when an entity is
/// selected AND the panel isn't collapsed.
///
/// 右岛揭示态——与"绑哪个实体"(随选区)分开。collapsed 是 sticky 用户偏好:选新实体会重绑终端但不强制重开
/// (优于 demo 每次重开;浮层头 panel-right 钮 + 动词 CTA 负责重开)。壳在"有选中 且 未收起"时揭示右岛。
class RightPanelCollapsed extends Notifier<bool> {
  @override
  bool build() => false;

  void toggle() => state = !state;
  void set(bool collapsed) => state = collapsed;
}

final rightPanelCollapsedProvider =
    NotifierProvider<RightPanelCollapsed, bool>(RightPanelCollapsed.new);
