import 'package:freezed_annotation/freezed_annotation.dart';

import '../../../../core/contract/entities/workflow.dart';

part 'run_terminal_state.freezed.dart';

/// The run lifecycle — a small state machine the terminal header renders (the streamed body lives in a
/// separate [CoalescingNotifier], NOT here, so a delta firehose never churns this Riverpod state).
/// idle = form shown, not yet run; running = in flight; ok/failed/cancelled = terminal.
/// 运行生命周期(小状态机,终端头渲染它;流式 body 在另一个 CoalescingNotifier、不在此,故 delta 风暴不搅这份态)。
enum RunPhase { idle, running, ok, failed, cancelled }

/// One executable entity's run state. The controller is a FAMILY keyed by [EntityRef] (so each entity has
/// its OWN run state + SSE subscription + coalescer): a run started on entity A keeps streaming when the
/// user selects B (background continue-streaming) and is intact when they return to A. The entity is the
/// family key — NOT in this state. The terminal's reveal (open/collapsed) is a SEPARATE concern
/// (`rightPanelProvider`), and the form's draft input lives on the controller (so the header verb CTA can
/// trigger a run too). Only `method` is here (it swaps the visible fields → must rebuild the form).
///
/// 一个可执行实体的运行态。controller 是按 [EntityRef] 的 family(每实体独立运行态 + SSE 订阅 + coalescer):
/// 在 A 上发起的运行,切到 B 仍后台续流、切回 A 完好。实体是 family 键、不在本态。揭示(开/收)是另一回事
/// (rightPanelProvider);表单草稿在 controller 上(故头部动词 CTA 也能触发 run)。仅 method 在此(它换可见字段、须重建表单)。
@freezed
abstract class RunTerminalState with _$RunTerminalState {
  const factory RunTerminalState({
    @Default(RunPhase.idle) RunPhase phase,
    @Default('') String method, // handler: the selected method (drives which fields render) 选中方法
    Object? output, // fn/hd/ag result output 结果输出
    String? errorCode,
    String? errorMsg,
    String? inputError, // form validation (bad JSON in an object/array field) 入参校验错
    @Default(0) int elapsedMs,
    String? logs, // fn captured logs 函数日志
    @Default(0) int steps, // agent 步数
    @Default(0) int tokensIn,
    @Default(0) int tokensOut,
    String? flowrunId, // workflow 触发的 flowrun id
    @Default(<FlowrunNode>[]) List<FlowrunNode> flowNodes, // workflow durable node list 工作流节点(真相)
    @Default(0) int runSeq, // generation counter — a stale run's result is dropped 运行代号,陈旧结果丢弃
  }) = _RunTerminalState;

  const RunTerminalState._();

  bool get isRunning => phase == RunPhase.running;
  bool get isTerminal => phase == RunPhase.ok || phase == RunPhase.failed || phase == RunPhase.cancelled;
}
