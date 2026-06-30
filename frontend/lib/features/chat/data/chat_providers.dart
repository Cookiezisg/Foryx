import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/runtime.dart';
import 'chat_repository.dart';

/// The Chat feature's data seam, as a provider. Defaults to [LiveChatRepository] over the Phase-4.0
/// pipeline (apiClient); the zero-backend demo, the gallery, and every feature test override THIS ONE
/// provider with a [FixtureChatRepository] via ProviderScope — the whole feature swaps backends at a
/// single seam (mirrors [entityRepositoryProvider]).
///
/// Chat feature 的数据缝(provider)。默认 Live(接 apiClient);零后端 demo / gallery / 每个 feature 测试经
/// ProviderScope override 此唯一 provider 成 fixture——整 feature 单点切换后端(镜像 entityRepositoryProvider)。
final chatRepositoryProvider = Provider<ChatRepository>((ref) {
  return LiveChatRepository(api: ref.watch(apiClientProvider));
});
