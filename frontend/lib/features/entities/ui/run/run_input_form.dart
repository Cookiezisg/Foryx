import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../../core/contract/entities/values.dart';
import '../../../../core/design/colors.dart';
import '../../../../core/design/tokens.dart';
import '../../../../core/design/typography.dart';
import '../../../../core/ui/an_button.dart';
import '../../../../core/ui/an_callout.dart';
import '../../../../core/ui/an_dropdown.dart';
import '../../../../core/ui/an_input.dart';
import '../../../../core/ui/icons.dart';
import '../../../../i18n/strings.g.dart';
import '../../data/entity_kind.dart';
import '../../state/run/run_terminal_controller.dart';
import '../../state/selected_entity.dart';

/// The run terminal's typed input form — renders the bound entity's declared inputs as type-appropriate
/// controls (string/number → text, boolean → dropdown, object/array → JSON textarea), a method picker for
/// handlers (fields follow the selected method), and an optional JSON payload for workflows. The input is
/// written to the FAMILY controller's `draft` (so the header verb CTA can run without reaching into this
/// widget); the inputs are uncontrolled (they hold their own text), keyed by ref+method+name so a method
/// switch reseeds from the persisted draft. Coercion + validation happen in the controller on run.
///
/// run 终端的类型化入参表单——按声明类型渲控件;输入写入 family controller 的 draft(故头部动词 CTA 无需伸进本
/// widget 即可 run);输入框非受控(自持文本),按 ref+method+name 键,换方法时从持久草稿重播。强转/校验在 controller。
class RunInputForm extends ConsumerStatefulWidget {
  const RunInputForm({
    required this.entityRef,
    required this.inputs,
    required this.methods,
    required this.verbLabel,
    super.key,
  });

  final EntityRef entityRef;
  final List<Field> inputs; // fn/ag declared inputs (empty for hd/wf) fn/ag 声明入参
  final List<MethodSpec> methods; // hd methods (empty otherwise) hd 方法
  final String verbLabel; // Run / Call / Invoke / Trigger

  @override
  ConsumerState<RunInputForm> createState() => _RunInputFormState();
}

class _RunInputFormState extends ConsumerState<RunInputForm> {
  static const _payloadKey = '__payload__';

  @override
  void initState() {
    super.initState();
    // Default the handler method to the first one (drives which fields render). 默认选第一个方法。
    if (widget.entityRef.kind == EntityKind.handler && widget.methods.isNotEmpty) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        final c = ref.read(runTerminalProvider(widget.entityRef).notifier);
        if (ref.read(runTerminalProvider(widget.entityRef)).method.isEmpty) {
          c.setMethod(widget.methods.first.name);
        }
      });
    }
  }

  List<Field> get _fields => switch (widget.entityRef.kind) {
        EntityKind.handler => widget.methods
                .where((m) => m.name == ref.read(runTerminalProvider(widget.entityRef)).method)
                .firstOrNull
                ?.inputs ??
            const [],
        EntityKind.function || EntityKind.agent => widget.inputs,
        EntityKind.workflow => const [],
      };

  @override
  Widget build(BuildContext context) {
    final r = context.t.entities.run;
    final p = runTerminalProvider(widget.entityRef);
    final state = ref.watch(p);
    final c = ref.read(p.notifier);
    final fields = _fields;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.entityRef.kind == EntityKind.handler) ...[
          _label(context, r.method),
          const SizedBox(height: AnSpace.s4),
          AnDropdown<String>(
            block: true,
            value: state.method.isEmpty ? null : state.method,
            enabled: !state.isRunning,
            options: [
              for (final m in widget.methods)
                AnDropdownOption(value: m.name, label: m.name, meta: m.streaming ? r.streaming : null),
            ],
            onChanged: c.setMethod,
          ),
          const SizedBox(height: AnSpace.s12),
        ],
        if (widget.entityRef.kind == EntityKind.workflow)
          _payloadField(context, c, state.isRunning)
        else if (fields.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: AnSpace.s4),
            child: Text(r.noInputs, style: AnText.meta.copyWith(color: context.colors.inkFaint)),
          )
        else
          for (final f in fields) ...[_field(context, c, f, state), const SizedBox(height: AnSpace.s12)],
        if (state.inputError != null) ...[
          AnCallout(_inputErrorText(context, state.inputError!), severity: AnCalloutSeverity.danger),
          const SizedBox(height: AnSpace.s12),
        ],
        _runButton(context, c, state),
      ],
    );
  }

  Widget _field(BuildContext context, RunTerminalController c, Field f, dynamic state) {
    final key = ValueKey('${widget.entityRef}/${state.method}/${f.name}');
    final Widget input;
    if (f.type == 'boolean') {
      final r = context.t.entities.run;
      input = AnDropdown<bool>(
        key: key,
        block: true,
        value: c.draft[f.name] as bool?,
        enabled: !state.isRunning,
        options: [
          AnDropdownOption(value: true, label: r.boolTrue),
          AnDropdownOption(value: false, label: r.boolFalse),
        ],
        onChanged: (v) => c.setField(f.name, v),
      );
    } else {
      final multi = f.type == 'object' || f.type == 'array';
      input = AnInput(
        key: key,
        block: true,
        multiline: multi,
        mono: multi || f.type == 'number',
        enabled: !state.isRunning,
        initialValue: c.draft[f.name] as String?,
        onChanged: (v) => c.setField(f.name, v),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _label(context, f.name, type: f.type, desc: f.description),
        const SizedBox(height: AnSpace.s4),
        input,
      ],
    );
  }

  Widget _payloadField(BuildContext context, RunTerminalController c, bool busy) {
    final r = context.t.entities.run;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _label(context, r.payload),
        const SizedBox(height: AnSpace.s4),
        AnInput(
          key: ValueKey('${widget.entityRef}/$_payloadKey'),
          block: true,
          multiline: true,
          mono: true,
          enabled: !busy,
          initialValue: c.draft[_payloadKey] as String?,
          onChanged: (v) => c.setField(_payloadKey, v),
        ),
        const SizedBox(height: AnSpace.s12),
      ],
    );
  }

  Widget _label(BuildContext context, String name, {String? type, String? desc}) {
    final c = context.colors;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.baseline,
          textBaseline: TextBaseline.alphabetic,
          children: [
            Flexible(
              child: Text(name,
                  maxLines: 1, overflow: TextOverflow.ellipsis, style: AnText.strong.copyWith(color: c.ink)),
            ),
            if (type != null) ...[
              const SizedBox(width: AnSpace.s6),
              Text(type, style: AnText.meta.copyWith(color: c.inkFaint)),
            ],
          ],
        ),
        if (desc != null && desc.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: AnSpace.s2),
            child: Text(desc, style: AnText.meta.copyWith(color: c.inkMuted)),
          ),
      ],
    );
  }

  Widget _runButton(BuildContext context, RunTerminalController c, dynamic state) {
    final r = context.t.entities.run;
    if (state.isRunning) {
      return AnButton(label: r.cancel, block: true, onPressed: c.cancel);
    }
    return AnButton(
      label: state.isTerminal ? r.runAgain : widget.verbLabel,
      icon: AnIcons.run,
      variant: AnButtonVariant.primary,
      block: true,
      onPressed: c.run,
    );
  }

  // Map the controller's coercion error code → localized text. 把强转错误码映射成本地化文案。
  String _inputErrorText(BuildContext context, String code) {
    final r = context.t.entities.run;
    if (code == 'payloadInvalid') return r.payloadInvalid;
    if (code == 'payloadObject') return r.payloadObject;
    if (code.startsWith('field:')) return r.fieldInvalid(name: code.substring(6));
    return code;
  }
}
