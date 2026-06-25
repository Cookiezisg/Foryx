// The on-wire shape of one SSE event, shared by all three streams (messages /
// entities / notifications). Mirrors the backend's `streamWire` envelope
// (transport/httpapi/response/stream.go) 1:1; this file is the Dart projection of
// that contract and must stay byte-synced with it (CLAUDE.md #9).
//
// 一条 SSE 事件的线上形状,三流(messages / entities / notifications)共用。1:1 镜像
// 后端 streamWire 信封;本文件是该契约的 Dart 投影,须与之逐字同步(CLAUDE.md #9)。
//
// Durability is NOT a wire field. The bus stamps ephemeral frames (deltas, ticks,
// transient signals) with seq 0 and durable frames with a monotonic seq>0, and only
// durable frames carry an SSE `id:` line. So the client derives "is this replayable /
// should it advance the resume cursor" from `seq > 0` — never from the frame verb
// (a Signal may be either). This is the single durability rule on the client.
//
// durable 不是线缆字段。bus 给 ephemeral 帧(delta/tick/瞬时 signal)盖 seq 0、给 durable
// 帧盖单调 seq>0,且只有 durable 帧带 SSE `id:` 行。故客户端用 `seq > 0` 判"是否可重放 /
// 是否推进续传游标"——绝不靠 frame 动词(signal 两者皆可能)。这是客户端唯一的 durable 规则。
library;

/// The rendering/broadcast anchor an event acts on. For messages the kind is always
/// `conversation`; for entities it is the entity kind (function/handler/agent/…); for
/// notifications it is `notification`. The client self-filters the (unfiltered,
/// workspace-wide) feed by this scope.
///
/// 事件作用的渲染/广播锚点。messages 恒 `conversation`;entities 为实体 kind;
/// notifications 为 `notification`。客户端据此 scope 自滤(后端不过滤的)工作区级流。
class StreamScope {
  const StreamScope({required this.kind, this.id = ''});

  final String kind;
  final String id;

  /// The subscription key — `kind:id` — used to demux/self-filter.
  ///
  /// 订阅键——`kind:id`——用于 demux / 自滤。
  String get key => '$kind:$id';

  factory StreamScope.fromJson(Map<String, dynamic> json) => StreamScope(
        kind: json['kind'] as String? ?? '',
        id: json['id'] as String? ?? '',
      );

  @override
  bool operator ==(Object other) =>
      other is StreamScope && other.kind == kind && other.id == id;

  @override
  int get hashCode => Object.hash(kind, id);

  @override
  String toString() => 'StreamScope($key)';
}

/// A frame's payload: a producer-owned `type` discriminant + opaque JSON `content`.
/// The protocol deliberately does NOT enumerate node types — that vocabulary belongs
/// to each producing module (events.md: "Node.Type 词表由 producer 定…非穷举"). So
/// `type` is an OPEN string here (never sealed): a producer may emit a new tick kind
/// at any time and the parser must survive it. Consumers branch on the types they know
/// and ignore the rest.
///
/// 帧的 payload:producer 拥有的 `type` 判别 + 不透明 JSON `content`。协议刻意不枚举 node
/// 类型——词表归各 producer 模块。故此处 `type` 是开放字符串(永不 seal):producer 随时可发
/// 新 tick 类型,解析器必须容得下。消费方只 branch 自己认识的类型、忽略其余。
class StreamNode {
  const StreamNode({required this.type, this.content});

  final String type;
  final Map<String, dynamic>? content;

  factory StreamNode.fromJson(Map<String, dynamic> json) => StreamNode(
        type: json['type'] as String? ?? '',
        content: json['content'] as Map<String, dynamic>?,
      );

  @override
  String toString() => 'StreamNode($type)';
}

/// One operation on the rendering tree — a CLOSED union of four verbs. This IS sealed
/// (the backend's `frame.kind` is a closed set), so an exhaustive `switch` over
/// subtypes is compiler-checked.
///
/// 对渲染树的一次操作——四动词封闭联合。这个**是** sealed(后端 `frame.kind` 是封闭集),
/// 故对子类型的穷尽 `switch` 受编译器检查。
sealed class StreamFrame {
  const StreamFrame();

  factory StreamFrame.fromJson(Map<String, dynamic> json) {
    final kind = json['kind'] as String? ?? '';
    switch (kind) {
      case 'open':
        return FrameOpen(
          parentId: json['parentId'] as String?,
          node: StreamNode.fromJson(
              (json['node'] as Map<String, dynamic>?) ?? const {}),
        );
      case 'delta':
        return FrameDelta(chunk: json['chunk'] as String? ?? '');
      case 'close':
        final result = json['result'] as Map<String, dynamic>?;
        return FrameClose(
          status: json['status'] as String? ?? '',
          error: json['error'] as String?,
          result: result == null ? null : StreamNode.fromJson(result),
        );
      case 'signal':
        return FrameSignal(
          node: StreamNode.fromJson(
              (json['node'] as Map<String, dynamic>?) ?? const {}),
        );
      default:
        // Forward-compat: an unknown verb degrades to a no-op signal rather than
        // crashing the stream. (The closed union is the backend's; the client must
        // not die if it ever widens.)
        //
        // 前向兼容:未知动词降级为空 signal 而非崩流。(封闭联合是后端的;若它日后扩,客户端不该死。)
        return FrameSignal(node: StreamNode(type: 'unknown:$kind'));
    }
  }
}

/// Create a node. `parentId` empty/null = top-level; non-empty = nest under that node
/// (E3 — e.g. a subagent's message subtree under a tool_call block).
///
/// 创建节点。`parentId` 空/null = 顶层;非空 = 嵌套于该节点下(E3——如 tool_call 块下挂
/// subagent 的 message 子树)。
class FrameOpen extends StreamFrame {
  const FrameOpen({this.parentId, required this.node});
  final String? parentId;
  final StreamNode node;
}

/// Append a streaming chunk to an open node (token text / terminal output). Always
/// ephemeral (seq 0) — lossy by design; the durable Close result is the truth.
///
/// 给开着的节点追加流式 chunk(token 文本 / 终端输出)。恒 ephemeral(seq 0)——设计上可丢;
/// 耐久的 Close result 才是真相。
class FrameDelta extends StreamFrame {
  const FrameDelta({required this.chunk});
  final String chunk;
}

/// Terminate a node. `result`, when present, is the final content snapshot — the
/// reconnect source of truth for streamed nodes (deltas are lossy). `error` set only
/// when status == "error".
///
/// 结束节点。`result` 非空时为最终内容快照——流式节点的重连真相(delta 可丢)。`error`
/// 仅 status == "error" 时非空。
class FrameClose extends StreamFrame {
  const FrameClose({required this.status, this.result, this.error});
  final String status; // completed | error | cancelled
  final StreamNode? result;
  final String? error;
}

/// A one-shot broadcast that builds no tree node (entity changed, flowrun tick,
/// trigger fire, chat interaction, notification). Whether it is durable is read off
/// the envelope's seq, not from here.
///
/// 不建树节点的瞬时广播(实体变更、flowrun tick、trigger fire、chat interaction、通知)。
/// 是否 durable 看信封 seq、不在此判。
class FrameSignal extends StreamFrame {
  const FrameSignal({required this.node});
  final StreamNode node;
}

/// A bus-stamped, delivered stream event: an envelope around one [StreamFrame].
///
/// bus 盖章、已投递的流式事件:一个 [StreamFrame] 的信封。
class StreamEnvelope {
  const StreamEnvelope({
    required this.seq,
    required this.scope,
    required this.id,
    required this.frame,
  });

  /// Monotonic per stream; 0 ⟺ ephemeral (no replay, did not carry an `id:` line).
  ///
  /// 每流单调;0 ⟺ ephemeral(不 replay、未带 `id:` 行)。
  final int seq;
  final StreamScope scope;

  /// The tree-node id this frame operates on (distinct from [StreamScope.id]).
  ///
  /// 此帧所操作的树节点 id(区别于 [StreamScope.id])。
  final String id;
  final StreamFrame frame;

  /// True when this frame is replayable and should advance the resume cursor. The one
  /// durability rule on the client (see file header).
  ///
  /// 此帧可重放、应推进续传游标时为真。客户端唯一 durable 规则(见文件头)。
  bool get durable => seq > 0;

  factory StreamEnvelope.fromJson(Map<String, dynamic> json) => StreamEnvelope(
        seq: (json['seq'] as num?)?.toInt() ?? 0,
        scope:
            StreamScope.fromJson((json['scope'] as Map<String, dynamic>?) ?? const {}),
        id: json['id'] as String? ?? '',
        frame: StreamFrame.fromJson(
            (json['frame'] as Map<String, dynamic>?) ?? const {}),
      );
}
