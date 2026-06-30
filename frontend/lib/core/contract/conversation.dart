import 'package:freezed_annotation/freezed_annotation.dart';

part 'conversation.freezed.dart';
part 'conversation.g.dart';

/// A chat-thread container — the backend projection of `conversation.Conversation`, as the rail and
/// ocean see it on the wire (camelCase ↔ json_serializable, no rename maps; mirrors `references/`).
/// This is the LIST-row + identity shape the rail consumes; the heavier thread config the ocean edits
/// (systemPrompt / attachedDocuments / modelOverride / summary) is added when that surface lands —
/// json_serializable simply ignores those wire keys until then.
///
/// Three flags are SYSTEM-WRITE / wire-read-only (never sent in PATCH), each driving a rail status
/// dot: [isGenerating] (an assistant turn is in flight → blue pulse), [awaitingInput] (≥1 pending
/// human-in-loop interaction → amber), [hasUnread] (a completed reply not yet seen → green). The first
/// two are derived server-side per request; [hasUnread] is a persisted column. [archived] drives the
/// gray "archived" marker when the rail shows archived threads.
///
/// 对话线程容器——后端 `conversation.Conversation` 的投影,rail/ocean 在线缆上所见(camelCase ↔
/// json_serializable、无重命名表;镜像 references/)。这是 rail 消费的「列表行 + 身份」形状;ocean 编辑的更重
/// 线程配置(systemPrompt / attachedDocuments / modelOverride / summary)待那个面落地再加——在此之前
/// json_serializable 直接忽略那些线缆键。三个标志系统写、线缆只读(不进 PATCH),各驱动一个 rail 状态点:
/// isGenerating(在途 assistant 回合→蓝呼吸)、awaitingInput(≥1 待决人在环→琥珀)、hasUnread(完成的回复未看→绿)。
/// 前两个服务端逐请求派生;hasUnread 是持久列。archived 在 rail 显归档时驱动灰色标记。
@freezed
abstract class Conversation with _$Conversation {
  const factory Conversation({
    required String id,
    @Default('') String title,
    @Default(false) bool autoTitled,
    @Default(false) bool archived,
    @Default(false) bool pinned,
    required DateTime createdAt,
    required DateTime updatedAt,
    required DateTime lastMessageAt,
    @Default(false) bool isGenerating,
    @Default(false) bool awaitingInput,
    @Default(false) bool hasUnread,
  }) = _Conversation;

  factory Conversation.fromJson(Map<String, dynamic> json) =>
      _$ConversationFromJson(json);
}
