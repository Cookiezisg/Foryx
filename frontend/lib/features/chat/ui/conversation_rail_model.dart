import '../../../core/contract/conversation.dart';
import '../../../core/model/sidebar_model.dart';
import '../../../core/model/status_state.dart';

/// The rail's time buckets, in display order. Pinned threads always sit in [pinned] (regardless of
/// time); the rest fall into a time bucket by lastMessageAt. Archived threads (when shown) are NOT a
/// bucket — they interleave by time, marked only by the gray dot.
///
/// rail 时间桶,按显示序。置顶恒在 pinned(不论时间);其余按 lastMessageAt 落时间桶。归档(显示时)不是桶——按时间穿插、只靠灰点标记。
enum ConvBucket { pinned, today, yesterday, lastWeek, older }

/// Which bucket a conversation belongs to (pure; [now] passed for testability). Calendar-day based in
/// LOCAL time so "Today"/"Yesterday" match the user's wall calendar.
///
/// 某对话属哪个桶(纯;传 now 便于测)。按**本地**日历日,使「今天/昨天」对齐用户墙上日历。
ConvBucket conversationBucket(Conversation c, DateTime now) {
  if (c.pinned) return ConvBucket.pinned;
  final at = c.lastMessageAt.toLocal();
  final days = DateTime(now.year, now.month, now.day)
      .difference(DateTime(at.year, at.month, at.day))
      .inDays;
  if (days <= 0) return ConvBucket.today;
  if (days == 1) return ConvBucket.yesterday;
  if (days <= 7) return ConvBucket.lastWeek;
  return ConvBucket.older;
}

/// The i18n strings for the relative-time row meta — injected (not read from slang) so the formatter
/// stays pure + unit-testable without a Translations object. The widget binds these from `t.chat.time`.
///
/// 相对时间行 meta 的 i18n 串——注入(不直读 slang),使格式化纯、可单测、不依赖 Translations。widget 从 t.chat.time 绑。
class ConvTimeStrings {
  const ConvTimeStrings({
    required this.justNow,
    required this.yesterday,
    required this.minutesAgo,
    required this.hoursAgo,
    required this.daysAgo,
  });

  final String justNow;
  final String yesterday;
  final String Function(int n) minutesAgo;
  final String Function(int n) hoursAgo;
  final String Function(int n) daysAgo;
}

/// The relative-time label for a row (just now / N min / N hr / yesterday / N days / a numeric date for
/// older). Calendar-day based in LOCAL time; older than 7 days → `y/m/d` (locale-neutral numerics).
///
/// 行的相对时间(刚刚/N 分钟/N 小时/昨天/N 天/更老用数字日期)。本地日历日;>7 天 → `年/月/日`(纯数字、无 locale 文本)。
String conversationTimeLabel(DateTime atUtc, DateTime now, ConvTimeStrings s) {
  final at = atUtc.toLocal();
  final days = DateTime(now.year, now.month, now.day)
      .difference(DateTime(at.year, at.month, at.day))
      .inDays;
  if (days <= 0) {
    final mins = now.difference(at).inMinutes;
    if (mins < 1) return s.justNow;
    if (mins < 60) return s.minutesAgo(mins);
    return s.hoursAgo(now.difference(at).inHours);
  }
  if (days == 1) return s.yesterday;
  if (days <= 7) return s.daysAgo(days);
  return '${at.year}/${at.month}/${at.day}';
}

/// All the i18n labels the rail model needs — the New/filter chrome, the bucket heads, and the time
/// strings. Bundled so the pure builder takes one struct (mirrors entities' RailLabels).
///
/// rail 模型需的全部 i18n 标签——New/过滤 chrome、桶头、时间串。打包成一个 struct 喂纯 builder(镜像 entities RailLabels)。
class ConvRailLabels {
  const ConvRailLabels({
    required this.newLabel,
    required this.filter,
    required this.pinned,
    required this.today,
    required this.yesterday,
    required this.lastWeek,
    required this.older,
    required this.time,
  });

  final String newLabel;
  final String filter;
  final String pinned;
  final String today;
  final String yesterday;
  final String lastWeek;
  final String older;
  final ConvTimeStrings time;

  String bucketLabel(ConvBucket b) => switch (b) {
        ConvBucket.pinned => pinned,
        ConvBucket.today => today,
        ConvBucket.yesterday => yesterday,
        ConvBucket.lastWeek => lastWeek,
        ConvBucket.older => older,
      };
}

/// Project the loaded conversations onto a [SidebarModel] for [AnSidebarList]. Each row = {id, title,
/// relative-time meta, lead dot}. When [groupByTime] is on → ONE [SidebarGroup] holding one collapsible
/// [SidebarType] PER non-empty bucket in [ConvBucket] order (置顶 / 今天 / 昨天 / 过去7天 / 更早) — the SAME
/// section-head treatment the entities rail uses (an AnRow collapsible head: label + count right-aligned
/// at the far edge + a lead disclosure chevron), NOT a bespoke big-group head, so the two rails read
/// identically and the count sits where the row timestamps do. `count` is set explicitly so a future
/// "show counts" toggle is just `count: null`. When off → one flat headless type, server order preserved.
/// Single-domain: NO per-kind section beyond the time buckets, NO client-side sort (the server orders via
/// ConvSort → ?sort=); rows are partitioned in arrival order.
///
/// 把已加载对话投影成 SidebarModel 喂 AnSidebarList。每行={id, 标题, 相对时间 meta, 前导点}。groupByTime 开 → 一个
/// SidebarGroup 内每个非空桶一个可折叠 SidebarType(按 ConvBucket 序 置顶/今天/昨天/过去7天/更早)——与 entities rail **同款**
/// 分节头(AnRow 可折叠头:label + 计数右对齐到最右缘 + 前导 chevron),非自造大组头,故两 rail 一致、计数与行时间戳同列。
/// count 显式设,使未来「显示计数」开关只需 count:null。关 → 单个扁平 headless type,保服务端序。单域:除时间桶无 per-kind 分节、无客户端排序。
SidebarModel buildConversationRailModel(
  List<Conversation> rows, {
  required DateTime now,
  required bool groupByTime,
  required ConvRailLabels labels,
}) {
  SidebarRow toRow(Conversation c) => SidebarRow(
        id: c.id,
        label: c.title,
        meta: conversationTimeLabel(c.lastMessageAt, now, labels.time),
        dot: conversationDot(c),
      );

  if (!groupByTime) {
    return SidebarModel(
      newLabel: labels.newLabel,
      filterPlaceholder: labels.filter,
      groups: [
        SidebarGroup(types: [SidebarType(rows: [for (final c in rows) toRow(c)])]),
      ],
    );
  }

  // Partition into buckets, preserving the server's order within each bucket. 按桶分组,桶内保服务端序。
  final byBucket = <ConvBucket, List<Conversation>>{};
  for (final c in rows) {
    (byBucket[conversationBucket(c, now)] ??= []).add(c);
  }
  // ONE group (label null → flattens) holding one collapsible TYPE per bucket — mirrors the entities rail
  // (one group, N typed sections). 一个组(label 空→扁平)持每桶一个可折叠 type——镜像 entities rail。
  return SidebarModel(
    newLabel: labels.newLabel,
    filterPlaceholder: labels.filter,
    groups: [
      SidebarGroup(types: [
        for (final b in ConvBucket.values)
          if ((byBucket[b] ?? const []).isNotEmpty)
            SidebarType(
              label: labels.bucketLabel(b),
              count: byBucket[b]!.length,
              rows: [for (final c in byBucket[b]!) toRow(c)],
            ),
      ]),
    ],
  );
}

/// The lead status dot for a conversation rail row — or null for a plain active thread (no dot, the
/// common case). Precedence, highest first:
///   generating (blue, the only animated/breathing dot) > awaiting input (amber "needs you") >
///   unread (green "answered while you were away") > archived (gray marker) > none.
/// The first three are the live activity signals (mutually exclusive in practice — a thread is
/// generating, or blocked on you, or has a fresh reply); the archived gray dot is a static "this is
/// archived" marker that only ever shows when the rail is set to include archived threads.
///
/// 会话 rail 行的前导状态点——普通活跃线程返 null(无点,常态)。优先级(高→低):生成中(蓝、唯一呼吸)>等你输入
/// (琥珀「等你」)>未读(绿「你不在时答完了」)>已归档(灰标记)>无。前三是活态信号(实际互斥);归档灰点是静态
/// 「这是归档」标记,仅当 rail 设为含归档时才出现。
AnStatus? conversationDot(Conversation c) {
  if (c.isGenerating) return AnStatus.run;
  if (c.awaitingInput) return AnStatus.wait;
  if (c.hasUnread) return AnStatus.done;
  if (c.archived) return AnStatus.idle;
  return null;
}
