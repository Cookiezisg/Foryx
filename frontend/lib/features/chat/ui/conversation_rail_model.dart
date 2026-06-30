import '../../../core/contract/conversation.dart';
import '../../../core/model/status_state.dart';

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
