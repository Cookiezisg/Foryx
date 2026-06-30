import '../../../core/contract/conversation.dart';
import 'chat_fixtures.dart';

/// Builds a realistic zero-backend [FixtureChatRepository] for `make demo` / the gallery — a spread of
/// conversations that exercises every rail signal at once: a pinned thread mid-generation (blue dot),
/// one awaiting your input (amber), one answered-but-unread (green), an archived one (gray when shown),
/// and a time spread (today / yesterday / this week / older) so the client-side buckets all populate.
/// Timestamps are relative to launch (DateTime.now) so the Today/Yesterday grouping reads correctly
/// whenever the demo is run.
///
/// 为 `make demo` / gallery 造一个真实的零后端 FixtureChatRepository——一组对话同时触发每种 rail 信号:置顶且
/// 生成中(蓝点)、等你输入(琥珀)、答完未读(绿)、一条归档(显示时灰),并铺开时间(今天/昨天/本周/更早)使前端时间桶都填上。
/// 时间相对启动(DateTime.now),故无论何时跑 demo,今天/昨天分组都读对。
FixtureChatRepository demoChatRepository() {
  final now = DateTime.now().toUtc();
  DateTime ago(Duration d) => now.subtract(d);

  Conversation conv(
    String id,
    String title,
    Duration since, {
    bool pinned = false,
    bool archived = false,
    bool generating = false,
    bool awaiting = false,
    bool unread = false,
  }) {
    final at = ago(since);
    return Conversation(
      id: id,
      title: title,
      autoTitled: true,
      pinned: pinned,
      archived: archived,
      createdAt: at.subtract(const Duration(minutes: 5)),
      updatedAt: at,
      lastMessageAt: at,
      isGenerating: generating,
      awaitingInput: awaiting,
      hasUnread: unread,
    );
  }

  return FixtureChatRepository(conversations: [
    conv('cv_daily', '竞品日报流程', const Duration(minutes: 2), pinned: true, generating: true),
    conv('cv_sync', 'AI 编辑 · sync_inventory 加重试', const Duration(minutes: 10)),
    conv('cv_diag', '诊断 · flowrun frn_8a1c 失败', const Duration(minutes: 25), awaiting: true),
    conv('cv_weekly', '周报初稿整理', const Duration(hours: 1), unread: true),
    conv('cv_keys', 'API key 轮换排查', const Duration(hours: 3)),
    conv('cv_notes', '周会纪要整理', const Duration(hours: 26)), // yesterday
    conv('cv_research', '市场调研问题清单', const Duration(days: 3)), // this week
    conv('cv_kickoff', '项目启动 kickoff 讨论', const Duration(days: 20)), // older
    conv('cv_migrate', '旧版迁移笔记', const Duration(days: 40), archived: true), // gray when shown
  ]);
}
