// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'conversation.dart';

// **************************************************************************
// JsonSerializableGenerator
// **************************************************************************

_Conversation _$ConversationFromJson(Map<String, dynamic> json) =>
    _Conversation(
      id: json['id'] as String,
      title: json['title'] as String? ?? '',
      autoTitled: json['autoTitled'] as bool? ?? false,
      archived: json['archived'] as bool? ?? false,
      pinned: json['pinned'] as bool? ?? false,
      createdAt: DateTime.parse(json['createdAt'] as String),
      updatedAt: DateTime.parse(json['updatedAt'] as String),
      lastMessageAt: DateTime.parse(json['lastMessageAt'] as String),
      isGenerating: json['isGenerating'] as bool? ?? false,
      awaitingInput: json['awaitingInput'] as bool? ?? false,
      hasUnread: json['hasUnread'] as bool? ?? false,
    );

Map<String, dynamic> _$ConversationToJson(_Conversation instance) =>
    <String, dynamic>{
      'id': instance.id,
      'title': instance.title,
      'autoTitled': instance.autoTitled,
      'archived': instance.archived,
      'pinned': instance.pinned,
      'createdAt': instance.createdAt.toIso8601String(),
      'updatedAt': instance.updatedAt.toIso8601String(),
      'lastMessageAt': instance.lastMessageAt.toIso8601String(),
      'isGenerating': instance.isGenerating,
      'awaitingInput': instance.awaitingInput,
      'hasUnread': instance.hasUnread,
    };
