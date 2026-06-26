import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/contract/api_error.dart';
import '../../../../core/contract/entities/values.dart';
import '../../../../core/messages/block_tree_reducer.dart';
import '../../../../core/perf/coalescing_notifier.dart';
import '../../../../core/sse/frame.dart';
import '../../data/entity_kind.dart';
import '../../data/entity_providers.dart';
import '../../data/entity_repository.dart';
import '../detail/entity_detail.dart';
import '../detail/entity_detail_provider.dart';
import '../selected_entity.dart';
import 'run_terminal_state.dart';

/// The high-frequency streamed body of one entity's terminal, held OUTSIDE Riverpod (in a
/// [CoalescingNotifier]) so a stderr/token firehose repaints the body leaf ≤1×/frame and never churns the
/// lifecycle state. fn/hd append the run-node deltas to [text]; agent folds the block frames into [tree];
/// workflow stamps live flowrun ticks into [liveNodes]. 一个实体终端的高频流式 body(coalescer 之内)。
class RunStream {
  final BlockTreeReducer tree = BlockTreeReducer();
  final StringBuffer _text = StringBuffer();
  final Map<String, String> liveNodes = {};

  String get text => _text.toString();
  void appendText(String s) => _text.write(s);

  void reset() {
    tree.clear();
    _text.clear();
    liveNodes.clear();
  }
}

/// The run terminal for ONE executable entity ([entityRef]) — a FAMILY keyed by [EntityRef]. Each member
/// owns its own panel SSE subscription + streamed body + lifecycle, so a run keeps streaming in the
/// BACKGROUND when the user selects another entity ([ref.keepAlive] is taken the moment a run starts, so
/// the member survives deselection) and is intact when they return. The verb CTA (header) and the form's
/// run button both call [run] — the typed input DRAFT lives here (so the header can trigger a run without
/// reaching into the form), coerced to the request on [run] using the entity's declared [Field] types.
/// Cancel ABANDONS the UI wait (sync verbs aren't abortable; the backend run completes + records its row).
///
/// 一个可执行实体的 run 终端(按 [EntityRef] 的 family)。各成员自管面板 SSE 订阅 + 流式 body + 生命周期,
/// 故一个运行在切走后台续流(run 起即 keepAlive、存活过取消选中)、切回完好。头部动词 CTA 与表单按钮都调
/// run——类型化输入草稿在此(头部无需伸进表单即可触发),run 时按声明的 Field 类型强转成请求。
class RunTerminalController extends Notifier<RunTerminalState> {
  RunTerminalController(this.entityRef);

  final EntityRef entityRef;
  late EntityRepository _repo;
  StreamSubscription<StreamEnvelope>? _panelSub;
  bool _keptAlive = false; // a run took a keep-alive (background streaming survives deselection) 已取 keepAlive

  final CoalescingNotifier<RunStream> stream = CoalescingNotifier(RunStream());

  /// The current form input (raw values: String for text/number/object/array, bool for boolean), kept off
  /// [state] so typing never rebuilds the lifecycle. Persists per entity (family + keepAlive). 当前表单草稿。
  final Map<String, Object?> draft = {};

  @override
  RunTerminalState build() {
    _repo = ref.watch(entityRepositoryProvider);
    _panelSub = _repo.panelSignals(entityRef.kind.scope(entityRef.id)).listen(_onPanel);
    ref.onDispose(() {
      _panelSub?.cancel();
      stream.dispose();
    });
    return const RunTerminalState();
  }

  /// Form text/bool field write — draft only, no rebuild (the inputs are uncontrolled). 表单字段写入(不重建)。
  void setField(String name, Object? value) => draft[name] = value;

  /// Handler method pick — in [state] because it swaps which fields render. 方法选择(在 state、换字段)。
  void setMethod(String method) {
    if (state.method != method) state = state.copyWith(method: method, inputError: null);
  }

  /// Execute the verb for this entity using the current draft (the header CTA + the form button both land
  /// here). Coerces draft → request by declared field type, captures execution-time stream frames, and
  /// finalizes from the result. Takes a keep-alive so the run survives deselection (background streaming).
  /// 用当前草稿执行本实体动词(头钮 + 表单按钮都到这)。按字段类型强转、捕获执行期帧、从结果收尾;取 keepAlive 后台续流。
  Future<void> run() async {
    final (request, inputError) = _coerce();
    if (inputError != null) {
      state = state.copyWith(inputError: inputError);
      return;
    }
    if (!_keptAlive) {
      ref.keepAlive(); // survive deselection so the run keeps streaming in the background 后台续流
      _keptAlive = true;
    }
    final seq = state.runSeq + 1;
    stream.mutate((s) => s..reset());
    state = state.copyWith(
      phase: RunPhase.running,
      runSeq: seq,
      inputError: null,
      output: null,
      errorCode: null,
      errorMsg: null,
      logs: null,
      steps: 0,
      tokensIn: 0,
      tokensOut: 0,
      flowrunId: null,
      flowNodes: const [],
    );
    final args = Map<String, dynamic>.from(request);
    try {
      switch (entityRef.kind) {
        case EntityKind.function:
          final r = await _repo.runFunction(entityRef.id, args: args);
          if (state.runSeq != seq) return;
          state = state.copyWith(
            phase: r.ok ? RunPhase.ok : RunPhase.failed,
            output: r.output,
            errorMsg: r.errorMsg.isEmpty ? null : r.errorMsg,
            elapsedMs: r.elapsedMs,
            logs: r.logs,
          );
        case EntityKind.handler:
          final r = await _repo.callHandler(entityRef.id, method: state.method, args: args);
          if (state.runSeq != seq) return;
          state = state.copyWith(phase: RunPhase.ok, output: r);
        case EntityKind.agent:
          final r = await _repo.invokeAgent(entityRef.id, input: args);
          if (state.runSeq != seq) return;
          state = state.copyWith(
            phase: r.ok ? RunPhase.ok : RunPhase.failed,
            output: r.output,
            errorMsg: (r.errorMsg ?? '').isEmpty ? null : r.errorMsg,
            elapsedMs: r.elapsedMs,
            steps: r.steps,
            tokensIn: r.tokensIn,
            tokensOut: r.tokensOut,
          );
        case EntityKind.workflow:
          final flowrunId =
              await _repo.triggerWorkflow(entityRef.id, payload: args.isEmpty ? null : args);
          if (state.runSeq != seq) return;
          state = state.copyWith(flowrunId: flowrunId);
          final comp = await _repo.getFlowrun(flowrunId);
          if (state.runSeq != seq) return;
          state = state.copyWith(
            phase: comp.flowrun.status == 'failed' ? RunPhase.failed : RunPhase.ok,
            flowNodes: comp.nodes,
            errorMsg: comp.flowrun.error,
          );
      }
    } on ApiException catch (e) {
      if (state.runSeq != seq) return;
      state = state.copyWith(phase: RunPhase.failed, errorCode: e.code, errorMsg: e.message);
    } catch (e) {
      if (state.runSeq != seq) return;
      state = state.copyWith(phase: RunPhase.failed, errorMsg: e.toString());
    }
  }

  /// Abandon the UI-side wait (the in-flight result is dropped via the seq bump; the backend run still
  /// completes + records its audit row). 放弃前端等待(后端续跑落审计行)。
  void cancel() => state = state.copyWith(phase: RunPhase.cancelled, runSeq: state.runSeq + 1);

  // Coerce the draft into the request by the entity's declared field types. workflow = one optional JSON
  // payload; fn/ag/hd = per-field (object/array via jsonDecode, surfacing a parse error). 草稿→请求强转。
  (Map<String, Object?>, String?) _coerce() {
    final detail = ref.read(entityDetailProvider(entityRef)).value;
    if (entityRef.kind == EntityKind.workflow) {
      final raw = (draft['__payload__'] as String?)?.trim() ?? '';
      if (raw.isEmpty) return (const {}, null);
      final Object? decoded;
      try {
        decoded = jsonDecode(raw);
      } catch (_) {
        return (const {}, 'payloadInvalid');
      }
      if (decoded is! Map<String, dynamic>) return (const {}, 'payloadObject');
      return (decoded, null);
    }
    final req = <String, Object?>{};
    for (final f in _fieldsFor(detail)) {
      if (f.type == 'boolean') {
        final b = draft[f.name];
        if (b is bool) req[f.name] = b;
        continue;
      }
      final raw = (draft[f.name] as String?)?.trim() ?? '';
      if (raw.isEmpty) continue;
      switch (f.type) {
        case 'number':
          req[f.name] = num.tryParse(raw) ?? raw;
        case 'object' || 'array':
          try {
            req[f.name] = jsonDecode(raw);
          } catch (_) {
            return (const {}, 'field:${f.name}');
          }
        default:
          req[f.name] = raw;
      }
    }
    return (req, null);
  }

  List<Field> _fieldsFor(EntityDetail? d) {
    if (d == null) return const [];
    return switch (entityRef.kind) {
      EntityKind.function => d.function?.activeVersion?.inputs ?? const [],
      EntityKind.agent => d.agent?.activeVersion?.inputs ?? const [],
      EntityKind.handler => d.handler?.activeVersion?.methods
              .where((m) => m.name == state.method)
              .firstOrNull
              ?.inputs ??
          const [],
      EntityKind.workflow => const [],
    };
  }

  void _onPanel(StreamEnvelope env) {
    switch (entityRef.kind) {
      case EntityKind.function:
      case EntityKind.handler:
        final f = env.frame;
        if (f is FrameDelta) stream.mutate((s) => s..appendText(f.chunk));
      case EntityKind.agent:
        stream.mutate((s) => s..tree.apply(env));
      case EntityKind.workflow:
        final f = env.frame;
        if (f is FrameSignal) {
          final c = f.node.content;
          final nodeId = c?['nodeId'] as String?;
          final status = c?['status'] as String?;
          if (nodeId != null && status != null) {
            stream.mutate((s) => s..liveNodes[nodeId] = status);
          }
        }
    }
  }
}

/// One run-terminal controller PER executable entity (family) — each keeps its own run alive in the
/// background. The right island shows the SELECTED entity's controller. 每可执行实体一个 controller(family)。
final runTerminalProvider =
    NotifierProvider.family<RunTerminalController, RunTerminalState, EntityRef>(RunTerminalController.new);
