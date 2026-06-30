import 'package:anselm/core/contract/conversation.dart';
import 'package:anselm/core/model/status_state.dart';
import 'package:anselm/features/chat/ui/conversation_rail_model.dart';
import 'package:flutter_test/flutter_test.dart';

// STEP 3 gate — the conversation-row lead-dot mapping. The row itself is a plain AnRow (verified
// visually in the gallery's Chat category); this pins the precedence that picks WHICH dot:
// generating > awaiting > unread > archived > none.

Conversation _c({bool generating = false, bool awaiting = false, bool unread = false, bool archived = false}) {
  final t = DateTime.utc(2026, 6, 26);
  return Conversation(
    id: 'cv_1',
    title: 't',
    createdAt: t,
    updatedAt: t,
    lastMessageAt: t,
    isGenerating: generating,
    awaitingInput: awaiting,
    hasUnread: unread,
    archived: archived,
  );
}

void main() {
  test('a plain active thread has no dot', () {
    expect(conversationDot(_c()), isNull);
  });

  test('generating → run (blue), the highest precedence', () {
    expect(conversationDot(_c(generating: true)), AnStatus.run);
    // wins even when every flag is set at once
    expect(conversationDot(_c(generating: true, awaiting: true, unread: true, archived: true)), AnStatus.run);
  });

  test('awaiting input → wait (amber), over unread + archived', () {
    expect(conversationDot(_c(awaiting: true)), AnStatus.wait);
    expect(conversationDot(_c(awaiting: true, unread: true, archived: true)), AnStatus.wait);
  });

  test('unread → done (green), over archived', () {
    expect(conversationDot(_c(unread: true)), AnStatus.done);
    expect(conversationDot(_c(unread: true, archived: true)), AnStatus.done);
  });

  test('archived → idle (gray marker), the lowest', () {
    expect(conversationDot(_c(archived: true)), AnStatus.idle);
  });
}
