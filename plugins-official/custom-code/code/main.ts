// custom-code 2.0 — the administrator's raw HTML in <head> and before </body> of every public page.
// Scope "head-scripts": the output is inserted as is (like the built-in plugin; only administrators can
// change the settings). Settings keep the built-in plugin's keys: headCode, bodyCode (Plugins → Custom code →
// Settings, up to 100,000 characters each).

/** A setting of this plugin ('' when not set). */
function setting(key: 'headCode' | 'bodyCode'): string {
  const v = site.settings(key);
  return v === null || v === undefined ? '' : String(v);
}

site.head(() => setting('headCode'));
site.bodyEnd(() => setting('bodyCode'));

/**
 * The built-in plugin had no tables: its settings (same keys) are copied by the core migration.
 * Nothing to import — answers { imported: 0 } for every batch.
 */
site.export('legacyImport', (_payload: unknown) => ({ imported: 0 }));
