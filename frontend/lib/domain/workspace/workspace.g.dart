// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'workspace.dart';

// **************************************************************************
// JsonSerializableGenerator
// **************************************************************************

_ModelRef _$ModelRefFromJson(Map<String, dynamic> json) => _ModelRef(
  apiKeyId: json['apiKeyId'] as String,
  modelId: json['modelId'] as String,
  options:
      (json['options'] as Map<String, dynamic>?)?.map(
        (k, e) => MapEntry(k, e as String),
      ) ??
      const <String, String>{},
);

Map<String, dynamic> _$ModelRefToJson(_ModelRef instance) => <String, dynamic>{
  'apiKeyId': instance.apiKeyId,
  'modelId': instance.modelId,
  'options': instance.options,
};

_Workspace _$WorkspaceFromJson(Map<String, dynamic> json) => _Workspace(
  id: json['id'] as String,
  name: json['name'] as String,
  avatarColor: json['avatarColor'] as String?,
  language: json['language'] as String,
  defaultDialogue: json['defaultDialogue'] == null
      ? null
      : ModelRef.fromJson(json['defaultDialogue'] as Map<String, dynamic>),
  defaultUtility: json['defaultUtility'] == null
      ? null
      : ModelRef.fromJson(json['defaultUtility'] as Map<String, dynamic>),
  defaultAgent: json['defaultAgent'] == null
      ? null
      : ModelRef.fromJson(json['defaultAgent'] as Map<String, dynamic>),
  defaultSearchKeyId: json['defaultSearchKeyId'] as String?,
  webFetchMode: json['webFetchMode'] as String?,
  lastUsedAt: json['lastUsedAt'] == null
      ? null
      : DateTime.parse(json['lastUsedAt'] as String),
  createdAt: DateTime.parse(json['createdAt'] as String),
  updatedAt: DateTime.parse(json['updatedAt'] as String),
);

Map<String, dynamic> _$WorkspaceToJson(_Workspace instance) =>
    <String, dynamic>{
      'id': instance.id,
      'name': instance.name,
      'avatarColor': instance.avatarColor,
      'language': instance.language,
      'defaultDialogue': instance.defaultDialogue,
      'defaultUtility': instance.defaultUtility,
      'defaultAgent': instance.defaultAgent,
      'defaultSearchKeyId': instance.defaultSearchKeyId,
      'webFetchMode': instance.webFetchMode,
      'lastUsedAt': instance.lastUsedAt?.toIso8601String(),
      'createdAt': instance.createdAt.toIso8601String(),
      'updatedAt': instance.updatedAt.toIso8601String(),
    };
