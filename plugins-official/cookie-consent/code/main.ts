// cookie-consent 2.0 — the banner is the package's bodyEnd template (templates/banner.html): it reads the
// settings of the Plugins page directly ({{settings.*}}, every value HTML-escaped, colours validated), so
// the plugin needs no code scope at all. The visitor's choice stays in localStorage.anasaki_consent
// ("accepted" | "declined") and is announced with the "anasaki:consent" event, as with the built-in plugin.

/**
 * The built-in plugin had no tables: its settings (same keys) are copied by the core migration.
 * Nothing to import — answers { imported: 0 } for every batch.
 */
site.export('legacyImport', (_payload: unknown) => ({ imported: 0 }));
