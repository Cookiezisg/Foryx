import 'package:freezed_annotation/freezed_annotation.dart';

part 'workspace.freezed.dart';
part 'workspace.g.dart';

/// A model selection (which API key + which model id + provider options). The backend's
/// `model.ModelRef`; carried by a workspace's three scenario defaults.
///
/// 一个模型选择(哪个 API key + 哪个 model id + provider 选项)。后端 `model.ModelRef`;
/// 由 workspace 三场景默认携带。
@freezed
abstract class ModelRef with _$ModelRef {
  const factory ModelRef({
    required String apiKeyId,
    required String modelId,
    @Default(<String, String>{}) Map<String, String> options,
  }) = _ModelRef;

  factory ModelRef.fromJson(Map<String, dynamic> json) =>
      _$ModelRefFromJson(json);
}

/// The local isolation unit — and the only auth axis (no accounts; the active workspace
/// id rides every request as `X-Forgify-Workspace-ID`). The first freezed DTO; the rest
/// of the entity types follow this exact pattern (camelCase wire ↔ json_serializable,
/// no rename maps). Mirrors backend `workspace.Workspace`.
///
/// 本地隔离单元——也是唯一鉴权轴(无账号;活动 workspace id 经 `X-Forgify-Workspace-ID` 随每请求)。
/// 首个 freezed DTO;其余实体型循此模式(camelCase 线缆 ↔ json_serializable、无重命名表)。
/// 镜像后端 `workspace.Workspace`。
@freezed
abstract class Workspace with _$Workspace {
  const factory Workspace({
    required String id,
    required String name,
    String? avatarColor,
    required String language,
    ModelRef? defaultDialogue,
    ModelRef? defaultUtility,
    ModelRef? defaultAgent,
    String? defaultSearchKeyId,
    String? webFetchMode, // local | jina
    DateTime? lastUsedAt,
    required DateTime createdAt,
    required DateTime updatedAt,
  }) = _Workspace;

  factory Workspace.fromJson(Map<String, dynamic> json) =>
      _$WorkspaceFromJson(json);
}
