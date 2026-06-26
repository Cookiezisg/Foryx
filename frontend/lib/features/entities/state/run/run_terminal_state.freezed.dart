// GENERATED CODE - DO NOT MODIFY BY HAND
// coverage:ignore-file
// ignore_for_file: type=lint
// ignore_for_file: unused_element, deprecated_member_use, deprecated_member_use_from_same_package, use_function_type_syntax_for_parameters, unnecessary_const, avoid_init_to_null, invalid_override_different_default_values_named, prefer_expression_function_bodies, annotate_overrides, invalid_annotation_target, unnecessary_question_mark

part of 'run_terminal_state.dart';

// **************************************************************************
// FreezedGenerator
// **************************************************************************

// dart format off
T _$identity<T>(T value) => value;
/// @nodoc
mixin _$RunTerminalState {

 RunPhase get phase; String get method;// handler: the selected method (drives which fields render) 选中方法
 Object? get output;// fn/hd/ag result output 结果输出
 String? get errorCode; String? get errorMsg; String? get inputError;// form validation (bad JSON in an object/array field) 入参校验错
 int get elapsedMs; String? get logs;// fn captured logs 函数日志
 int get steps;// agent 步数
 int get tokensIn; int get tokensOut; String? get flowrunId;// workflow 触发的 flowrun id
 List<FlowrunNode> get flowNodes;// workflow durable node list 工作流节点(真相)
 int get runSeq;
/// Create a copy of RunTerminalState
/// with the given fields replaced by the non-null parameter values.
@JsonKey(includeFromJson: false, includeToJson: false)
@pragma('vm:prefer-inline')
$RunTerminalStateCopyWith<RunTerminalState> get copyWith => _$RunTerminalStateCopyWithImpl<RunTerminalState>(this as RunTerminalState, _$identity);



@override
bool operator ==(Object other) {
  return identical(this, other) || (other.runtimeType == runtimeType&&other is RunTerminalState&&(identical(other.phase, phase) || other.phase == phase)&&(identical(other.method, method) || other.method == method)&&const DeepCollectionEquality().equals(other.output, output)&&(identical(other.errorCode, errorCode) || other.errorCode == errorCode)&&(identical(other.errorMsg, errorMsg) || other.errorMsg == errorMsg)&&(identical(other.inputError, inputError) || other.inputError == inputError)&&(identical(other.elapsedMs, elapsedMs) || other.elapsedMs == elapsedMs)&&(identical(other.logs, logs) || other.logs == logs)&&(identical(other.steps, steps) || other.steps == steps)&&(identical(other.tokensIn, tokensIn) || other.tokensIn == tokensIn)&&(identical(other.tokensOut, tokensOut) || other.tokensOut == tokensOut)&&(identical(other.flowrunId, flowrunId) || other.flowrunId == flowrunId)&&const DeepCollectionEquality().equals(other.flowNodes, flowNodes)&&(identical(other.runSeq, runSeq) || other.runSeq == runSeq));
}


@override
int get hashCode => Object.hash(runtimeType,phase,method,const DeepCollectionEquality().hash(output),errorCode,errorMsg,inputError,elapsedMs,logs,steps,tokensIn,tokensOut,flowrunId,const DeepCollectionEquality().hash(flowNodes),runSeq);

@override
String toString() {
  return 'RunTerminalState(phase: $phase, method: $method, output: $output, errorCode: $errorCode, errorMsg: $errorMsg, inputError: $inputError, elapsedMs: $elapsedMs, logs: $logs, steps: $steps, tokensIn: $tokensIn, tokensOut: $tokensOut, flowrunId: $flowrunId, flowNodes: $flowNodes, runSeq: $runSeq)';
}


}

/// @nodoc
abstract mixin class $RunTerminalStateCopyWith<$Res>  {
  factory $RunTerminalStateCopyWith(RunTerminalState value, $Res Function(RunTerminalState) _then) = _$RunTerminalStateCopyWithImpl;
@useResult
$Res call({
 RunPhase phase, String method, Object? output, String? errorCode, String? errorMsg, String? inputError, int elapsedMs, String? logs, int steps, int tokensIn, int tokensOut, String? flowrunId, List<FlowrunNode> flowNodes, int runSeq
});




}
/// @nodoc
class _$RunTerminalStateCopyWithImpl<$Res>
    implements $RunTerminalStateCopyWith<$Res> {
  _$RunTerminalStateCopyWithImpl(this._self, this._then);

  final RunTerminalState _self;
  final $Res Function(RunTerminalState) _then;

/// Create a copy of RunTerminalState
/// with the given fields replaced by the non-null parameter values.
@pragma('vm:prefer-inline') @override $Res call({Object? phase = null,Object? method = null,Object? output = freezed,Object? errorCode = freezed,Object? errorMsg = freezed,Object? inputError = freezed,Object? elapsedMs = null,Object? logs = freezed,Object? steps = null,Object? tokensIn = null,Object? tokensOut = null,Object? flowrunId = freezed,Object? flowNodes = null,Object? runSeq = null,}) {
  return _then(_self.copyWith(
phase: null == phase ? _self.phase : phase // ignore: cast_nullable_to_non_nullable
as RunPhase,method: null == method ? _self.method : method // ignore: cast_nullable_to_non_nullable
as String,output: freezed == output ? _self.output : output ,errorCode: freezed == errorCode ? _self.errorCode : errorCode // ignore: cast_nullable_to_non_nullable
as String?,errorMsg: freezed == errorMsg ? _self.errorMsg : errorMsg // ignore: cast_nullable_to_non_nullable
as String?,inputError: freezed == inputError ? _self.inputError : inputError // ignore: cast_nullable_to_non_nullable
as String?,elapsedMs: null == elapsedMs ? _self.elapsedMs : elapsedMs // ignore: cast_nullable_to_non_nullable
as int,logs: freezed == logs ? _self.logs : logs // ignore: cast_nullable_to_non_nullable
as String?,steps: null == steps ? _self.steps : steps // ignore: cast_nullable_to_non_nullable
as int,tokensIn: null == tokensIn ? _self.tokensIn : tokensIn // ignore: cast_nullable_to_non_nullable
as int,tokensOut: null == tokensOut ? _self.tokensOut : tokensOut // ignore: cast_nullable_to_non_nullable
as int,flowrunId: freezed == flowrunId ? _self.flowrunId : flowrunId // ignore: cast_nullable_to_non_nullable
as String?,flowNodes: null == flowNodes ? _self.flowNodes : flowNodes // ignore: cast_nullable_to_non_nullable
as List<FlowrunNode>,runSeq: null == runSeq ? _self.runSeq : runSeq // ignore: cast_nullable_to_non_nullable
as int,
  ));
}

}


/// Adds pattern-matching-related methods to [RunTerminalState].
extension RunTerminalStatePatterns on RunTerminalState {
/// A variant of `map` that fallback to returning `orElse`.
///
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case final Subclass value:
///     return ...;
///   case _:
///     return orElse();
/// }
/// ```

@optionalTypeArgs TResult maybeMap<TResult extends Object?>(TResult Function( _RunTerminalState value)?  $default,{required TResult orElse(),}){
final _that = this;
switch (_that) {
case _RunTerminalState() when $default != null:
return $default(_that);case _:
  return orElse();

}
}
/// A `switch`-like method, using callbacks.
///
/// Callbacks receives the raw object, upcasted.
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case final Subclass value:
///     return ...;
///   case final Subclass2 value:
///     return ...;
/// }
/// ```

@optionalTypeArgs TResult map<TResult extends Object?>(TResult Function( _RunTerminalState value)  $default,){
final _that = this;
switch (_that) {
case _RunTerminalState():
return $default(_that);case _:
  throw StateError('Unexpected subclass');

}
}
/// A variant of `map` that fallback to returning `null`.
///
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case final Subclass value:
///     return ...;
///   case _:
///     return null;
/// }
/// ```

@optionalTypeArgs TResult? mapOrNull<TResult extends Object?>(TResult? Function( _RunTerminalState value)?  $default,){
final _that = this;
switch (_that) {
case _RunTerminalState() when $default != null:
return $default(_that);case _:
  return null;

}
}
/// A variant of `when` that fallback to an `orElse` callback.
///
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case Subclass(:final field):
///     return ...;
///   case _:
///     return orElse();
/// }
/// ```

@optionalTypeArgs TResult maybeWhen<TResult extends Object?>(TResult Function( RunPhase phase,  String method,  Object? output,  String? errorCode,  String? errorMsg,  String? inputError,  int elapsedMs,  String? logs,  int steps,  int tokensIn,  int tokensOut,  String? flowrunId,  List<FlowrunNode> flowNodes,  int runSeq)?  $default,{required TResult orElse(),}) {final _that = this;
switch (_that) {
case _RunTerminalState() when $default != null:
return $default(_that.phase,_that.method,_that.output,_that.errorCode,_that.errorMsg,_that.inputError,_that.elapsedMs,_that.logs,_that.steps,_that.tokensIn,_that.tokensOut,_that.flowrunId,_that.flowNodes,_that.runSeq);case _:
  return orElse();

}
}
/// A `switch`-like method, using callbacks.
///
/// As opposed to `map`, this offers destructuring.
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case Subclass(:final field):
///     return ...;
///   case Subclass2(:final field2):
///     return ...;
/// }
/// ```

@optionalTypeArgs TResult when<TResult extends Object?>(TResult Function( RunPhase phase,  String method,  Object? output,  String? errorCode,  String? errorMsg,  String? inputError,  int elapsedMs,  String? logs,  int steps,  int tokensIn,  int tokensOut,  String? flowrunId,  List<FlowrunNode> flowNodes,  int runSeq)  $default,) {final _that = this;
switch (_that) {
case _RunTerminalState():
return $default(_that.phase,_that.method,_that.output,_that.errorCode,_that.errorMsg,_that.inputError,_that.elapsedMs,_that.logs,_that.steps,_that.tokensIn,_that.tokensOut,_that.flowrunId,_that.flowNodes,_that.runSeq);case _:
  throw StateError('Unexpected subclass');

}
}
/// A variant of `when` that fallback to returning `null`
///
/// It is equivalent to doing:
/// ```dart
/// switch (sealedClass) {
///   case Subclass(:final field):
///     return ...;
///   case _:
///     return null;
/// }
/// ```

@optionalTypeArgs TResult? whenOrNull<TResult extends Object?>(TResult? Function( RunPhase phase,  String method,  Object? output,  String? errorCode,  String? errorMsg,  String? inputError,  int elapsedMs,  String? logs,  int steps,  int tokensIn,  int tokensOut,  String? flowrunId,  List<FlowrunNode> flowNodes,  int runSeq)?  $default,) {final _that = this;
switch (_that) {
case _RunTerminalState() when $default != null:
return $default(_that.phase,_that.method,_that.output,_that.errorCode,_that.errorMsg,_that.inputError,_that.elapsedMs,_that.logs,_that.steps,_that.tokensIn,_that.tokensOut,_that.flowrunId,_that.flowNodes,_that.runSeq);case _:
  return null;

}
}

}

/// @nodoc


class _RunTerminalState extends RunTerminalState {
  const _RunTerminalState({this.phase = RunPhase.idle, this.method = '', this.output, this.errorCode, this.errorMsg, this.inputError, this.elapsedMs = 0, this.logs, this.steps = 0, this.tokensIn = 0, this.tokensOut = 0, this.flowrunId, final  List<FlowrunNode> flowNodes = const <FlowrunNode>[], this.runSeq = 0}): _flowNodes = flowNodes,super._();
  

@override@JsonKey() final  RunPhase phase;
@override@JsonKey() final  String method;
// handler: the selected method (drives which fields render) 选中方法
@override final  Object? output;
// fn/hd/ag result output 结果输出
@override final  String? errorCode;
@override final  String? errorMsg;
@override final  String? inputError;
// form validation (bad JSON in an object/array field) 入参校验错
@override@JsonKey() final  int elapsedMs;
@override final  String? logs;
// fn captured logs 函数日志
@override@JsonKey() final  int steps;
// agent 步数
@override@JsonKey() final  int tokensIn;
@override@JsonKey() final  int tokensOut;
@override final  String? flowrunId;
// workflow 触发的 flowrun id
 final  List<FlowrunNode> _flowNodes;
// workflow 触发的 flowrun id
@override@JsonKey() List<FlowrunNode> get flowNodes {
  if (_flowNodes is EqualUnmodifiableListView) return _flowNodes;
  // ignore: implicit_dynamic_type
  return EqualUnmodifiableListView(_flowNodes);
}

// workflow durable node list 工作流节点(真相)
@override@JsonKey() final  int runSeq;

/// Create a copy of RunTerminalState
/// with the given fields replaced by the non-null parameter values.
@override @JsonKey(includeFromJson: false, includeToJson: false)
@pragma('vm:prefer-inline')
_$RunTerminalStateCopyWith<_RunTerminalState> get copyWith => __$RunTerminalStateCopyWithImpl<_RunTerminalState>(this, _$identity);



@override
bool operator ==(Object other) {
  return identical(this, other) || (other.runtimeType == runtimeType&&other is _RunTerminalState&&(identical(other.phase, phase) || other.phase == phase)&&(identical(other.method, method) || other.method == method)&&const DeepCollectionEquality().equals(other.output, output)&&(identical(other.errorCode, errorCode) || other.errorCode == errorCode)&&(identical(other.errorMsg, errorMsg) || other.errorMsg == errorMsg)&&(identical(other.inputError, inputError) || other.inputError == inputError)&&(identical(other.elapsedMs, elapsedMs) || other.elapsedMs == elapsedMs)&&(identical(other.logs, logs) || other.logs == logs)&&(identical(other.steps, steps) || other.steps == steps)&&(identical(other.tokensIn, tokensIn) || other.tokensIn == tokensIn)&&(identical(other.tokensOut, tokensOut) || other.tokensOut == tokensOut)&&(identical(other.flowrunId, flowrunId) || other.flowrunId == flowrunId)&&const DeepCollectionEquality().equals(other._flowNodes, _flowNodes)&&(identical(other.runSeq, runSeq) || other.runSeq == runSeq));
}


@override
int get hashCode => Object.hash(runtimeType,phase,method,const DeepCollectionEquality().hash(output),errorCode,errorMsg,inputError,elapsedMs,logs,steps,tokensIn,tokensOut,flowrunId,const DeepCollectionEquality().hash(_flowNodes),runSeq);

@override
String toString() {
  return 'RunTerminalState(phase: $phase, method: $method, output: $output, errorCode: $errorCode, errorMsg: $errorMsg, inputError: $inputError, elapsedMs: $elapsedMs, logs: $logs, steps: $steps, tokensIn: $tokensIn, tokensOut: $tokensOut, flowrunId: $flowrunId, flowNodes: $flowNodes, runSeq: $runSeq)';
}


}

/// @nodoc
abstract mixin class _$RunTerminalStateCopyWith<$Res> implements $RunTerminalStateCopyWith<$Res> {
  factory _$RunTerminalStateCopyWith(_RunTerminalState value, $Res Function(_RunTerminalState) _then) = __$RunTerminalStateCopyWithImpl;
@override @useResult
$Res call({
 RunPhase phase, String method, Object? output, String? errorCode, String? errorMsg, String? inputError, int elapsedMs, String? logs, int steps, int tokensIn, int tokensOut, String? flowrunId, List<FlowrunNode> flowNodes, int runSeq
});




}
/// @nodoc
class __$RunTerminalStateCopyWithImpl<$Res>
    implements _$RunTerminalStateCopyWith<$Res> {
  __$RunTerminalStateCopyWithImpl(this._self, this._then);

  final _RunTerminalState _self;
  final $Res Function(_RunTerminalState) _then;

/// Create a copy of RunTerminalState
/// with the given fields replaced by the non-null parameter values.
@override @pragma('vm:prefer-inline') $Res call({Object? phase = null,Object? method = null,Object? output = freezed,Object? errorCode = freezed,Object? errorMsg = freezed,Object? inputError = freezed,Object? elapsedMs = null,Object? logs = freezed,Object? steps = null,Object? tokensIn = null,Object? tokensOut = null,Object? flowrunId = freezed,Object? flowNodes = null,Object? runSeq = null,}) {
  return _then(_RunTerminalState(
phase: null == phase ? _self.phase : phase // ignore: cast_nullable_to_non_nullable
as RunPhase,method: null == method ? _self.method : method // ignore: cast_nullable_to_non_nullable
as String,output: freezed == output ? _self.output : output ,errorCode: freezed == errorCode ? _self.errorCode : errorCode // ignore: cast_nullable_to_non_nullable
as String?,errorMsg: freezed == errorMsg ? _self.errorMsg : errorMsg // ignore: cast_nullable_to_non_nullable
as String?,inputError: freezed == inputError ? _self.inputError : inputError // ignore: cast_nullable_to_non_nullable
as String?,elapsedMs: null == elapsedMs ? _self.elapsedMs : elapsedMs // ignore: cast_nullable_to_non_nullable
as int,logs: freezed == logs ? _self.logs : logs // ignore: cast_nullable_to_non_nullable
as String?,steps: null == steps ? _self.steps : steps // ignore: cast_nullable_to_non_nullable
as int,tokensIn: null == tokensIn ? _self.tokensIn : tokensIn // ignore: cast_nullable_to_non_nullable
as int,tokensOut: null == tokensOut ? _self.tokensOut : tokensOut // ignore: cast_nullable_to_non_nullable
as int,flowrunId: freezed == flowrunId ? _self.flowrunId : flowrunId // ignore: cast_nullable_to_non_nullable
as String?,flowNodes: null == flowNodes ? _self._flowNodes : flowNodes // ignore: cast_nullable_to_non_nullable
as List<FlowrunNode>,runSeq: null == runSeq ? _self.runSeq : runSeq // ignore: cast_nullable_to_non_nullable
as int,
  ));
}


}

// dart format on
