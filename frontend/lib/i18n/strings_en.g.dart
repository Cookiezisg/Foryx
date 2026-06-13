///
/// Generated file. Do not edit.
///
// coverage:ignore-file
// ignore_for_file: type=lint, unused_import
// dart format off

part of 'strings.g.dart';

// Path: <root>
typedef TranslationsEn = Translations; // ignore: unused_element
class Translations with BaseTranslations<AppLocale, Translations> {
	/// Returns the current translations of the given [context].
	///
	/// Usage:
	/// final t = Translations.of(context);
	static Translations of(BuildContext context) => InheritedLocaleData.of<AppLocale, Translations>(context).translations;

	/// You can call this constructor and build your own translation instance of this locale.
	/// Constructing via the enum [AppLocale.build] is preferred.
	Translations({Map<String, Node>? overrides, PluralResolver? cardinalResolver, PluralResolver? ordinalResolver, TranslationMetadata<AppLocale, Translations>? meta})
		: assert(overrides == null, 'Set "translation_overrides: true" in order to enable this feature.'),
		  $meta = meta ?? TranslationMetadata(
		    locale: AppLocale.en,
		    overrides: overrides ?? {},
		    cardinalResolver: cardinalResolver,
		    ordinalResolver: ordinalResolver,
		  ) {
		$meta.setFlatMapFunction(_flatMapFunction);
	}

	/// Metadata for the translations of <en>.
	@override final TranslationMetadata<AppLocale, Translations> $meta;

	/// Access flat map
	dynamic operator[](String key) => $meta.getTranslation(key);

	late final Translations _root = this; // ignore: unused_field

	Translations $copyWith({TranslationMetadata<AppLocale, Translations>? meta}) => Translations(meta: meta ?? this.$meta);

	// Translations
	late final Translations$app$en app = Translations$app$en._(_root);
	late final Translations$backend$en backend = Translations$backend$en._(_root);
	late final Translations$workspace$en workspace = Translations$workspace$en._(_root);
	late final Translations$nav$en nav = Translations$nav$en._(_root);
}

// Path: app
class Translations$app$en {
	Translations$app$en._(this._root);

	final Translations _root; // ignore: unused_field

	// Translations

	/// en: 'Forgify'
	String get name => 'Forgify';
}

// Path: backend
class Translations$backend$en {
	Translations$backend$en._(this._root);

	final Translations _root; // ignore: unused_field

	// Translations

	/// en: 'Starting Forgify…'
	String get starting => 'Starting Forgify…';

	/// en: 'Backend failed to start'
	String get crashedTitle => 'Backend failed to start';

	/// en: 'Retry'
	String get retry => 'Retry';
}

// Path: workspace
class Translations$workspace$en {
	Translations$workspace$en._(this._root);

	final Translations _root; // ignore: unused_field

	// Translations

	/// en: 'Select a workspace'
	String get selectTitle => 'Select a workspace';

	/// en: 'No workspace selected'
	String get none => 'No workspace selected';
}

// Path: nav
class Translations$nav$en {
	Translations$nav$en._(this._root);

	final Translations _root; // ignore: unused_field

	// Translations

	/// en: 'Chat'
	String get chat => 'Chat';

	/// en: 'Functions'
	String get functions => 'Functions';

	/// en: 'Handlers'
	String get handlers => 'Handlers';

	/// en: 'Agents'
	String get agents => 'Agents';

	/// en: 'Workflows'
	String get workflows => 'Workflows';

	/// en: 'Search'
	String get search => 'Search';

	/// en: 'Settings'
	String get settings => 'Settings';
}

/// The flat map containing all translations for locale <en>.
/// Only for edge cases! For simple maps, use the map function of this library.
///
/// The Dart AOT compiler has issues with very large switch statements,
/// so the map is split into smaller functions (512 entries each).
extension on Translations {
	dynamic _flatMapFunction(String path) {
		return switch (path) {
			'app.name' => 'Forgify',
			'backend.starting' => 'Starting Forgify…',
			'backend.crashedTitle' => 'Backend failed to start',
			'backend.retry' => 'Retry',
			'workspace.selectTitle' => 'Select a workspace',
			'workspace.none' => 'No workspace selected',
			'nav.chat' => 'Chat',
			'nav.functions' => 'Functions',
			'nav.handlers' => 'Handlers',
			'nav.agents' => 'Agents',
			'nav.workflows' => 'Workflows',
			'nav.search' => 'Search',
			'nav.settings' => 'Settings',
			_ => null,
		};
	}
}
