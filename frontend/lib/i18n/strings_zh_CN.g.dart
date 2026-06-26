///
/// Generated file. Do not edit.
///
// coverage:ignore-file
// ignore_for_file: type=lint, unused_import
// dart format off

import 'package:flutter/widgets.dart';
import 'package:intl/intl.dart';
import 'package:slang/generated.dart';
import 'strings.g.dart';

// Path: <root>
class TranslationsZhCn extends Translations with BaseTranslations<AppLocale, Translations> {
	/// You can call this constructor and build your own translation instance of this locale.
	/// Constructing via the enum [AppLocale.build] is preferred.
	TranslationsZhCn({Map<String, Node>? overrides, PluralResolver? cardinalResolver, PluralResolver? ordinalResolver, TranslationMetadata<AppLocale, Translations>? meta})
		: assert(overrides == null, 'Set "translation_overrides: true" in order to enable this feature.'),
		  $meta = meta ?? TranslationMetadata(
		    locale: AppLocale.zhCn,
		    overrides: overrides ?? {},
		    cardinalResolver: cardinalResolver,
		    ordinalResolver: ordinalResolver,
		  ),
		  super(cardinalResolver: cardinalResolver, ordinalResolver: ordinalResolver) {
		super.$meta.setFlatMapFunction($meta.getTranslation); // copy base translations to super.$meta
		$meta.setFlatMapFunction(_flatMapFunction);
	}

	/// Metadata for the translations of <zh-CN>.
	@override final TranslationMetadata<AppLocale, Translations> $meta;

	/// Access flat map
	@override dynamic operator[](String key) => $meta.getTranslation(key) ?? super.$meta.getTranslation(key);

	late final TranslationsZhCn _root = this; // ignore: unused_field

	@override 
	TranslationsZhCn $copyWith({TranslationMetadata<AppLocale, Translations>? meta}) => TranslationsZhCn(meta: meta ?? this.$meta);

	// Translations
	@override String get appName => 'Anselm';
	@override late final _Translations$status$zh_CN status = _Translations$status$zh_CN._(_root);
	@override late final _Translations$action$zh_CN action = _Translations$action$zh_CN._(_root);
	@override late final _Translations$feedback$zh_CN feedback = _Translations$feedback$zh_CN._(_root);
	@override late final _Translations$ref$zh_CN ref = _Translations$ref$zh_CN._(_root);
	@override late final _Translations$a11y$zh_CN a11y = _Translations$a11y$zh_CN._(_root);
	@override late final _Translations$diff$zh_CN diff = _Translations$diff$zh_CN._(_root);
	@override late final _Translations$tree$zh_CN tree = _Translations$tree$zh_CN._(_root);
	@override late final _Translations$startup$zh_CN startup = _Translations$startup$zh_CN._(_root);
	@override late final _Translations$entities$zh_CN entities = _Translations$entities$zh_CN._(_root);
}

// Path: status
class _Translations$status$zh_CN extends Translations$status$en {
	_Translations$status$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get idle => '空闲';
	@override String get run => '运行中';
	@override String get wait => '等待';
	@override String get err => '失败';
	@override String get done => '完成';
}

// Path: action
class _Translations$action$zh_CN extends Translations$action$en {
	_Translations$action$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get edit => '编辑';
	@override String get cancel => '取消';
	@override String get save => '保存';
	@override String get copy => '复制';
	@override String get wrap => '自动换行';
	@override String get delete => '删除';
}

// Path: feedback
class _Translations$feedback$zh_CN extends Translations$feedback$en {
	_Translations$feedback$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get info => '提示';
	@override String get success => '成功';
	@override String get warning => '警告';
	@override String get error => '错误';
	@override String get dismiss => '关闭';
	@override String get confirmDelete => '确认删除';
	@override String get dialogBarrier => '关闭对话框';
	@override String get loading => '加载中';
	@override String stepOf({required Object n, required Object m}) => '第 ${n} 步 / 共 ${m} 步';
	@override String goToStep({required Object n}) => '跳到第 ${n} 步';
	@override String removeTag({required Object name}) => '移除 ${name}';
	@override String get addTag => '添加标签';
	@override String get copied => '已复制';
	@override String get copyFailed => '复制失败';
}

// Path: ref
class _Translations$ref$zh_CN extends Translations$ref$en {
	_Translations$ref$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get function => '函数';
	@override String get handler => '处理器';
	@override String get workflow => '工作流';
	@override String get agent => '智能体';
	@override String get document => '文档';
	@override String get conversation => '会话';
	@override String get skill => '技能';
	@override String get mcp => 'MCP';
	@override String get trigger => '触发器';
	@override String get control => '控制';
	@override String get approval => '审批';
}

// Path: a11y
class _Translations$a11y$zh_CN extends Translations$a11y$en {
	_Translations$a11y$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String editingField({required Object field}) => '正在编辑 ${field}';
	@override String get displayOptions => '显示选项';
	@override String codeBlock({required Object lang, required Object lines}) => '代码块,${lang},${lines} 行';
	@override String codeBlockPlain({required Object lines}) => '代码块,${lines} 行';
	@override String jsonTree({required Object count}) => 'JSON 树,${count} 项';
	@override String diff({required Object added, required Object removed}) => '差异,新增 ${added},删除 ${removed}';
}

// Path: diff
class _Translations$diff$zh_CN extends Translations$diff$en {
	_Translations$diff$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get added => '新增';
	@override String get removed => '删除';
}

// Path: tree
class _Translations$tree$zh_CN extends Translations$tree$en {
	_Translations$tree$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get invalidJson => '无效 JSON';
	@override String get circular => '[循环引用]';
	@override String moreItems({required Object count}) => '${count} 项已省略';
}

// Path: startup
class _Translations$startup$zh_CN extends Translations$startup$en {
	_Translations$startup$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get connecting => '正在连接本地引擎…';
	@override String get crashedTitle => '无法连接本地引擎';
	@override String get crashedHint => '后端未启动。开发时把 ANSELM_BACKEND_URL 指向已运行的服务(make server)。';
	@override String get retry => '重试';
	@override String get errorTitle => '出错了';
	@override String get errorHint => '渲染此视图时发生了意外错误。';
}

// Path: entities
class _Translations$entities$zh_CN extends Translations$entities$en {
	_Translations$entities$zh_CN._(TranslationsZhCn root) : this._root = root, super.internal(root);

	final TranslationsZhCn _root; // ignore: unused_field

	// Translations
	@override String get kNew => '新建';
	@override String get filter => '筛选…';
	@override String get emptyTitle => '还没有实体';
	@override String get emptyHint => '新建一个函数、处理器、智能体或工作流来开始。';
	@override String get errorTitle => '无法加载实体';
	@override String get errorHint => '本地引擎没有返回实体列表。';
	@override String get retry => '重试';
	@override String get selectTitle => '选择一个实体';
	@override String get selectHint => '从左侧选择一个函数、处理器、智能体或工作流。';
	@override String get sortLabel => '排序';
	@override String get sortRecent => '最近更新';
	@override String get sortName => '名称';
}

/// The flat map containing all translations for locale <zh-CN>.
/// Only for edge cases! For simple maps, use the map function of this library.
///
/// The Dart AOT compiler has issues with very large switch statements,
/// so the map is split into smaller functions (512 entries each).
extension on TranslationsZhCn {
	dynamic _flatMapFunction(String path) {
		return switch (path) {
			'appName' => 'Anselm',
			'status.idle' => '空闲',
			'status.run' => '运行中',
			'status.wait' => '等待',
			'status.err' => '失败',
			'status.done' => '完成',
			'action.edit' => '编辑',
			'action.cancel' => '取消',
			'action.save' => '保存',
			'action.copy' => '复制',
			'action.wrap' => '自动换行',
			'action.delete' => '删除',
			'feedback.info' => '提示',
			'feedback.success' => '成功',
			'feedback.warning' => '警告',
			'feedback.error' => '错误',
			'feedback.dismiss' => '关闭',
			'feedback.confirmDelete' => '确认删除',
			'feedback.dialogBarrier' => '关闭对话框',
			'feedback.loading' => '加载中',
			'feedback.stepOf' => ({required Object n, required Object m}) => '第 ${n} 步 / 共 ${m} 步',
			'feedback.goToStep' => ({required Object n}) => '跳到第 ${n} 步',
			'feedback.removeTag' => ({required Object name}) => '移除 ${name}',
			'feedback.addTag' => '添加标签',
			'feedback.copied' => '已复制',
			'feedback.copyFailed' => '复制失败',
			'ref.function' => '函数',
			'ref.handler' => '处理器',
			'ref.workflow' => '工作流',
			'ref.agent' => '智能体',
			'ref.document' => '文档',
			'ref.conversation' => '会话',
			'ref.skill' => '技能',
			'ref.mcp' => 'MCP',
			'ref.trigger' => '触发器',
			'ref.control' => '控制',
			'ref.approval' => '审批',
			'a11y.editingField' => ({required Object field}) => '正在编辑 ${field}',
			'a11y.displayOptions' => '显示选项',
			'a11y.codeBlock' => ({required Object lang, required Object lines}) => '代码块,${lang},${lines} 行',
			'a11y.codeBlockPlain' => ({required Object lines}) => '代码块,${lines} 行',
			'a11y.jsonTree' => ({required Object count}) => 'JSON 树,${count} 项',
			'a11y.diff' => ({required Object added, required Object removed}) => '差异,新增 ${added},删除 ${removed}',
			'diff.added' => '新增',
			'diff.removed' => '删除',
			'tree.invalidJson' => '无效 JSON',
			'tree.circular' => '[循环引用]',
			'tree.moreItems' => ({required Object count}) => '${count} 项已省略',
			'startup.connecting' => '正在连接本地引擎…',
			'startup.crashedTitle' => '无法连接本地引擎',
			'startup.crashedHint' => '后端未启动。开发时把 ANSELM_BACKEND_URL 指向已运行的服务(make server)。',
			'startup.retry' => '重试',
			'startup.errorTitle' => '出错了',
			'startup.errorHint' => '渲染此视图时发生了意外错误。',
			'entities.kNew' => '新建',
			'entities.filter' => '筛选…',
			'entities.emptyTitle' => '还没有实体',
			'entities.emptyHint' => '新建一个函数、处理器、智能体或工作流来开始。',
			'entities.errorTitle' => '无法加载实体',
			'entities.errorHint' => '本地引擎没有返回实体列表。',
			'entities.retry' => '重试',
			'entities.selectTitle' => '选择一个实体',
			'entities.selectHint' => '从左侧选择一个函数、处理器、智能体或工作流。',
			'entities.sortLabel' => '排序',
			'entities.sortRecent' => '最近更新',
			'entities.sortName' => '名称',
			_ => null,
		};
	}
}
